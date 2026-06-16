package main

import (
	"fmt"

	"bastion/internal/store"
)

const (
	maxSnippetLabel = 256
	maxSnippetBody  = 65536
)

// --- Groups -----------------------------------------------------------------

// ListGroups returns all groups.
func (a *App) ListGroups() ([]store.Group, error) {
	if !a.IsUnlocked() {
		return nil, errLocked
	}
	return a.store.ListGroups()
}

// CreateGroup adds a new group.
func (a *App) CreateGroup(name string) (store.Group, error) {
	if !a.IsUnlocked() {
		return store.Group{}, errLocked
	}
	return a.store.CreateGroup(name)
}

// RenameGroup renames a group.
func (a *App) RenameGroup(id, name string) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	return a.store.RenameGroup(id, name)
}

// DeleteGroup removes a group (its hosts fall back to "Ungrouped").
func (a *App) DeleteGroup(id string) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	return a.store.DeleteGroup(id)
}

// --- Snippets ---------------------------------------------------------------

// ListSnippets returns all snippets.
func (a *App) ListSnippets() ([]store.Snippet, error) {
	if !a.IsUnlocked() {
		return nil, errLocked
	}
	return a.store.ListSnippets()
}

// CreateSnippet adds a new snippet.
func (a *App) CreateSnippet(label, body string) (store.Snippet, error) {
	if !a.IsUnlocked() {
		return store.Snippet{}, errLocked
	}
	if len(label) > maxSnippetLabel {
		return store.Snippet{}, fmt.Errorf("bastion: snippet label too long (%d max)", maxSnippetLabel)
	}
	if len(body) > maxSnippetBody {
		return store.Snippet{}, fmt.Errorf("bastion: snippet body too long (%d max)", maxSnippetBody)
	}
	return a.store.CreateSnippet(label, body)
}

// UpdateSnippet edits a snippet.
func (a *App) UpdateSnippet(id, label, body string) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	if len(label) > maxSnippetLabel {
		return fmt.Errorf("bastion: snippet label too long (%d max)", maxSnippetLabel)
	}
	if len(body) > maxSnippetBody {
		return fmt.Errorf("bastion: snippet body too long (%d max)", maxSnippetBody)
	}
	return a.store.UpdateSnippet(id, label, body)
}

// DeleteSnippet removes a snippet.
func (a *App) DeleteSnippet(id string) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	return a.store.DeleteSnippet(id)
}
