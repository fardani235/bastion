package main

import appssh "bastion/internal/ssh"

// SessionHealthDTO is the frontend-facing health info for sessions.
type SessionHealthDTO struct {
	Count   int    `json:"count"`
	Uptimes []appssh.SessionInfo `json:"uptimes,omitempty"`
}

// ListSessionHealth returns aggregate session health info.
func (a *App) ListSessionHealth() SessionHealthDTO {
	return SessionHealthDTO{
		Count:   a.sessions.Count(),
		Uptimes: a.sessions.ListSessions(),
	}
}
