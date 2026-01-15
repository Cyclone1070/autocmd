package workflow

import (
	"github.com/Cyclone1070/iav/internal/domain"
)

// SwitchSession changes the current session to an existing one.
func (w *Workflow) SwitchSession(id string) error {
	s, err := w.sessionStore.Get(id)
	if err != nil {
		return err
	}
	w.currentSession = s
	return nil
}

// NewSession creates a new session and sets it as current.
func (w *Workflow) NewSession() error {
	s, err := w.sessionStore.Create()
	if err != nil {
		return err
	}
	w.currentSession = s
	return nil
}

// DeleteSession removes a session by ID.
func (w *Workflow) DeleteSession(id string) error {
	if err := w.sessionStore.Delete(id); err != nil {
		return err
	}
	if w.currentSession != nil && w.currentSession.ID == id {
		w.currentSession = nil
	}
	return nil
}

// CurrentSession returns the currently active session.
func (w *Workflow) CurrentSession() *domain.Session {
	return w.currentSession
}

// ListSessions returns summaries of all available sessions.
func (w *Workflow) ListSessions() ([]domain.SessionSummary, error) {
	return w.sessionStore.List()
}
