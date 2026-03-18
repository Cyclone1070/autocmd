package workflow

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
)

type sessionPickerStore interface {
	List() ([]domain.SessionSummary, error)
	Create() (*domain.Session, error)
	Rename(id, name string) error
	Delete(id string) error
}

type sessionPickerState interface {
	CurrentSessionID() string
	SetCurrentSessionID(id string)
	Save() error
}

// SessionPickerWorkflow orchestrates session management operations.
type SessionPickerWorkflow struct {
	store sessionPickerStore
	state sessionPickerState
}

// NewSessionPickerWorkflow creates a new SessionPickerWorkflow.
func NewSessionPickerWorkflow(store sessionPickerStore, state sessionPickerState) *SessionPickerWorkflow {
	return &SessionPickerWorkflow{
		store: store,
		state: state,
	}
}

// PrepareSelection gathers the sessions and the current active session.
func (w *SessionPickerWorkflow) PrepareSelection(ctx context.Context) (*domain.SessionPickerSnapshot, error) {
	summaries, err := w.store.List()
	if err != nil {
		return nil, err
	}

	return &domain.SessionPickerSnapshot{
		Sessions:         summaries,
		CurrentSessionID: w.state.CurrentSessionID(),
	}, nil
}

// ApplySelection sets the active session in state.
func (w *SessionPickerWorkflow) ApplySelection(ctx context.Context, id string) error {
	w.state.SetCurrentSessionID(id)
	return w.state.Save()
}

// CreateSession creates a new session and sets it as active.
func (w *SessionPickerWorkflow) CreateSession(ctx context.Context) (string, error) {
	sess, err := w.store.Create()
	if err != nil {
		return "", err
	}

	w.state.SetCurrentSessionID(sess.ID)
	if err := w.state.Save(); err != nil {
		return "", err
	}

	return sess.ID, nil
}

// RenameSession renames an existing session.
func (w *SessionPickerWorkflow) RenameSession(ctx context.Context, id, name string) error {
	return w.store.Rename(id, name)
}

// DeleteSession removes a session and clears the active session state if it was deleted.
func (w *SessionPickerWorkflow) DeleteSession(ctx context.Context, id string) error {
	if err := w.store.Delete(id); err != nil {
		return err
	}

	if id == w.state.CurrentSessionID() {
		w.state.SetCurrentSessionID("")
		return w.state.Save()
	}

	return nil
}
