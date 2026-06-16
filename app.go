package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	appssh "bastion/internal/ssh"
	"bastion/internal/store"
	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound application object. It owns the SQLite store and,
// once unlocked, the in-memory vault key. All Wails IPC methods hang off this
// struct.
type App struct {
	ctx context.Context

	// mu guards key and store across IPC calls.
	mu    sync.Mutex
	store *store.Store
	key   []byte // 32 bytes when unlocked; nil otherwise.

	// knownHostsPath is the on-disk OpenSSH known_hosts file backing the SSH
	// trust store. Set in startup; overridden in tests.
	knownHostsPath string

	// sessions manages live SSH sessions. Created in startup once the ctx is
	// available so the emitter can publish Wails events.
	sessions *appssh.Manager

	// emitter is the Wails event emitter for session output.
	emitter *wailsEmitter

	// forwardManager manages port forwarding lifecycle across sessions.
	forwardManager *appssh.PortForwardManager

	// autoLockTimer fires after autoLockTimeout of inactivity. A dedicated
	// goroutine (started in startup) listens on its C channel instead of using
	// AfterFunc, so that touchAutoLock can safely stop/drain/reset without
	// racing against a previously-fired callback goroutine.
	autoLockTimer   *time.Timer
	autoLockTimeout time.Duration
	autoLockStop    chan struct{}

	screensaverConn     *dbus.Conn
	screensaverSignalCh chan *dbus.Signal
}

// NewApp constructs an empty App. The store is opened in startup so the data
// directory can be created and any open error surfaces during app boot.
func NewApp() *App {
	return &App{}
}

// startup is called by Wails after the runtime is ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	dir, err := dataDir()
	if err != nil {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Startup Error",
			Message: fmt.Sprintf("Could not determine data directory:\n\n%s", err),
		})
		os.Exit(1)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Startup Error",
			Message: fmt.Sprintf("Could not create data directory %s:\n\n%s", dir, err),
		})
		os.Exit(1)
	}

	s, err := store.Open(filepath.Join(dir, "vault.db"))
	if err != nil {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Startup Error",
			Message: fmt.Sprintf("Could not open vault database:\n\n%s", err),
		})
		os.Exit(1)
	}
	a.store = s
	a.knownHostsPath = filepath.Join(dir, "known_hosts")
	a.forwardManager = appssh.NewPortForwardManager()
	a.emitter = newWailsEmitter(ctx)
	a.emitter.logDir = filepath.Join(dir, "logs")
	a.sessions = appssh.NewManager(a.emitter, a.forwardManager)

	a.autoLockTimeout = 5 * time.Minute
	a.autoLockTimer = time.NewTimer(a.autoLockTimeout)
	a.autoLockStop = make(chan struct{})
	go a.autoLockLoop()

	a.watchScreenSaver()
}

// autoLockLoop listens for the auto-lock timer and locks the vault on expiry.
// Stopped by closing autoLockStop.
func (a *App) autoLockLoop() {
	for {
		select {
		case <-a.autoLockTimer.C:
			a.mu.Lock()
			wasUnlocked := a.key != nil
			if wasUnlocked {
				zero(a.key)
				a.key = nil
			}
			a.mu.Unlock()
			if wasUnlocked {
				// Tear down live terminals so the lock isn't merely cosmetic.
				if a.sessions != nil {
					a.sessions.CloseAll()
				}
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "vault:locked")
				}
			}
		case <-a.autoLockStop:
			return
		}
	}
}

// touchAutoLock resets the idle timer. Call from every user-driven IPC method
// that should count as activity.
func (a *App) touchAutoLock() {
	a.mu.Lock()
	if a.autoLockTimer != nil {
		if !a.autoLockTimer.Stop() {
			select {
			case <-a.autoLockTimer.C:
			default:
			}
		}
		a.autoLockTimer.Reset(a.autoLockTimeout)
	}
	a.mu.Unlock()
}

// GetAutoLockSeconds returns the current auto-lock idle timeout.
func (a *App) GetAutoLockSeconds() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return int(a.autoLockTimeout.Seconds())
}

// SetAutoLockSeconds sets the auto-lock idle timeout. Minimum 60 seconds.
func (a *App) SetAutoLockSeconds(secs int) error {
	if secs < 60 {
		return fmt.Errorf("bastion: auto-lock timeout must be at least 60 seconds")
	}
	a.mu.Lock()
	a.autoLockTimeout = time.Duration(secs) * time.Second
	if a.autoLockTimer != nil {
		if !a.autoLockTimer.Stop() {
			select {
			case <-a.autoLockTimer.C:
			default:
			}
		}
		a.autoLockTimer.Reset(a.autoLockTimeout)
	}
	a.mu.Unlock()
	return nil
}

// shutdown is called by Wails before exit. We zero the in-memory key and
// close the DB. Wails calls this on graceful quit.
func (a *App) shutdown(ctx context.Context) {
	if a.autoLockStop != nil {
		close(a.autoLockStop)
	}

	if a.sessions != nil {
		a.sessions.CloseAll()
	}

	if a.screensaverSignalCh != nil {
		close(a.screensaverSignalCh)
	}
	if a.screensaverConn != nil {
		_ = a.screensaverConn.Close()
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.autoLockTimer != nil {
		a.autoLockTimer.Stop()
	}

	zero(a.key)
	a.key = nil

	if a.store != nil {
		_ = a.store.Close()
	}
}

// dataDir returns the OS-appropriate config directory for Bastion. On Linux
// this is ~/.config/bastion.
func dataDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "bastion"), nil
}

// watchScreenSaver subscribes to the D-Bus ScreenSaver ActiveChanged signal
// so the vault locks automatically when the screen is locked (Linux only).
func (a *App) watchScreenSaver() {
	conn, err := dbus.SessionBus()
	if err != nil {
		return // not on Linux, no D-Bus, etc.
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.ScreenSaver"),
		dbus.WithMatchMember("ActiveChanged"),
	); err != nil {
		return // screen saver signals not available
	}
	ch := make(chan *dbus.Signal, 8)
	conn.Signal(ch)
	a.screensaverConn = conn
	a.screensaverSignalCh = ch
	go func() {
		for sig := range ch {
			if len(sig.Body) < 1 {
				continue
			}
			active, ok := sig.Body[0].(bool)
			if !ok || !active {
				continue
			}
			a.mu.Lock()
			wasUnlocked := a.key != nil
			if wasUnlocked {
				zero(a.key)
				a.key = nil
				if a.autoLockTimer != nil {
					a.autoLockTimer.Stop()
				}
			}
			a.mu.Unlock()
			if wasUnlocked {
				if a.sessions != nil {
					a.sessions.CloseAll()
				}
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "vault:locked")
				}
			}
		}
	}()
}

// zero overwrites b with zeros. Safe to call with nil.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
