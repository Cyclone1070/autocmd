package workflow

import (
	"github.com/Cyclone1070/iav/internal/domain"
)

// SwitchSession changes the current session to an existing one.
// If Run() is active, it will be cancelled and waited for before switching.
func (w *Workflow) SwitchSession(id string) error {
	// Cancel any running loop and wait for it to finish
	w.mu.Lock()
	cancel := w.runCancel
	done := w.runDone
	w.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}

	s, err := w.sessionStore.Get(id)
	if err != nil {
		return err
	}
	w.currentSession = s
	return nil
}

// NewSession creates a new session and sets it as current.
// If Run() is active, it will be cancelled and waited for before switching.
func (w *Workflow) NewSession() error {
	// Cancel any running loop and wait for it to finish
	w.mu.Lock()
	cancel := w.runCancel
	done := w.runDone
	w.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}

	s, err := w.sessionStore.Create()
	if err != nil {
		return err
	}
	w.currentSession = s
	return nil
}

// DeleteSession removes a session by ID.
// If Run() is active on the session being deleted, it will be cancelled first.
func (w *Workflow) DeleteSession(id string) error {
	// Cancel any running loop on this session and wait for it to finish
	if w.currentSession != nil && w.currentSession.ID == id {
		w.mu.Lock()
		cancel := w.runCancel
		done := w.runDone
		w.mu.Unlock()

		if cancel != nil {
			cancel()
			<-done
		}
	}

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

