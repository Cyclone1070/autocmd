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

type sessionPickerBus interface {
	SendUIUpdate(domain.UIUpdate)
	WorkflowActions() <-chan domain.Action
}

type SessionPickerDeps struct {
	Bus   sessionPickerBus
	Store sessionPickerStore
	State sessionPickerState
}

// RunSessionPicker starts the session management workflow asynchronously.
func RunSessionPicker(ctx context.Context, deps *SessionPickerDeps) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		wf := newSessionPickerWorkflow(deps.Store, deps.State)

		// 1. Send initial snapshot
		snapshot, err := wf.prepareSelection(ctx)
		if err != nil {
			done <- err
			return
		}
		deps.Bus.SendUIUpdate(snapshot)

		// 2. Loop for actions
		for {
			select {
			case <-ctx.Done():
				done <- ctx.Err()
				return
			case act, ok := <-deps.Bus.WorkflowActions():
				if !ok {
					done <- nil
					return
				}

				switch a := act.(type) {
				case domain.SelectSessionAction:
					if err := wf.applySelection(ctx, a.ID); err != nil {
						done <- err
						return
					}
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- nil
					return

				case domain.CreateSessionAction:
					if _, err := wf.createSession(ctx); err != nil {
						done <- err
						return
					}
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- nil
					return

				case domain.RenameSessionAction:
					if err := wf.renameSession(ctx, a.ID, a.Name); err != nil {
						done <- err
						return
					}

				case domain.DeleteSessionAction:
					if err := wf.deleteSession(ctx, a.ID); err != nil {
						done <- err
						return
					}

				case domain.StopAction:
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- nil
					return
				}

				// After any mutation (or if we need to refresh), send a new snapshot
				snapshot, err := wf.prepareSelection(ctx)
				if err != nil {
					done <- err
					return
				}
				deps.Bus.SendUIUpdate(snapshot)
			}
		}
	}()
	return done
}

// CreateSession is a public helper to create a new session and update state.
// Used by the 'iav session new' command.
func CreateSession(store sessionPickerStore, state sessionPickerState) (string, error) {
	wf := newSessionPickerWorkflow(store, state)
	return wf.createSession(context.Background())
}

type sessionPickerWorkflow struct {
	store sessionPickerStore
	state sessionPickerState
}

func newSessionPickerWorkflow(store sessionPickerStore, state sessionPickerState) *sessionPickerWorkflow {
	return &sessionPickerWorkflow{
		store: store,
		state: state,
	}
}

func (w *sessionPickerWorkflow) prepareSelection(ctx context.Context) (domain.SessionListEvent, error) {
	summaries, err := w.store.List()
	if err != nil {
		return domain.SessionListEvent{}, err
	}

	return domain.SessionListEvent{
		Sessions:         summaries,
		CurrentSessionID: w.state.CurrentSessionID(),
	}, nil
}

func (w *sessionPickerWorkflow) applySelection(ctx context.Context, id string) error {
	w.state.SetCurrentSessionID(id)
	return w.state.Save()
}

func (w *sessionPickerWorkflow) createSession(ctx context.Context) (string, error) {
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

func (w *sessionPickerWorkflow) renameSession(ctx context.Context, id, name string) error {
	return w.store.Rename(id, name)
}

func (w *sessionPickerWorkflow) deleteSession(ctx context.Context, id string) error {
	if err := w.store.Delete(id); err != nil {
		return err
	}

	if id == w.state.CurrentSessionID() {
		w.state.SetCurrentSessionID("")
		return w.state.Save()
	}

	return nil
}
