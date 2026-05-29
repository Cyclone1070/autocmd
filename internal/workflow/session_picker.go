package workflow

import (
	"context"
	"slices"

	"github.com/Cyclone1070/autocmd/internal/domain"
)

type sessionPickerStore interface {
	List() ([]domain.SessionMetadata, error)
	GetMetadata(id string) (*domain.SessionMetadata, error)
	Create(workingDir string) (*domain.Session, error)
	FindBlank(workingDir string) (*domain.SessionMetadata, error)
	Rename(id, name string) error
	Delete(id string) error
	SetActive(id, workingDir string) error
}

type sessionPickerBus interface {
	SendUIUpdate(domain.UIUpdate)
	WorkflowActions() <-chan domain.Action
}

// SessionPickerDeps contains the dependencies for the session selection workflow.
type SessionPickerDeps struct {
	Bus        sessionPickerBus
	Store      sessionPickerStore
	WorkingDir string
}

// RunSessionPicker starts the session management workflow asynchronously.
func RunSessionPicker(ctx context.Context, deps *SessionPickerDeps) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		wf := newSessionPickerWorkflow(deps.Store, deps.WorkingDir)

		// 1. Send initial snapshot
		snapshot, err := wf.prepareSelection()
		if err != nil {
			errCh <- err
			return
		}
		deps.Bus.SendUIUpdate(snapshot)

		// 2. Loop for actions
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case act, ok := <-deps.Bus.WorkflowActions():
				if !ok {
					errCh <- nil
					return
				}

				switch a := act.(type) {
				case domain.SelectSessionAction:
					targetDir, err := wf.applySelection(a.ID)
					if err != nil {
						errCh <- err
						return
					}
					deps.Bus.SendUIUpdate(domain.SessionSelectedEvent{
						ID:             a.ID,
						SwitchRequired: targetDir != "",
						TargetDir:      targetDir,
					})
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					errCh <- nil
					return

				case domain.CreateSessionAction:
					id, err := wf.createSession()
					if err != nil {
						errCh <- err
						return
					}
					// If the new session is successfully created, we don't switch cwd
					_ = id
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					errCh <- nil
					return

				case domain.RenameSessionAction:
					if err := wf.renameSession(a.ID, a.Name); err != nil {
						errCh <- err
						return
					}

				case domain.DeleteSessionAction:
					if err := wf.deleteSession(a.ID); err != nil {
						errCh <- err
						return
					}

				case domain.StopAction:
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					errCh <- context.Canceled
					return
				}

				// After any mutation (or if we need to refresh), send a new snapshot
				snapshot, err := wf.prepareSelection()
				if err != nil {
					errCh <- err
					return
				}
				deps.Bus.SendUIUpdate(snapshot)
			}
		}
	}()
	return errCh
}

// CreateSession is a public helper to create a new session and update state.
// Used by the 'autocmd session new' command.
func CreateSession(store sessionPickerStore, workingDir string) (string, error) {
	wf := newSessionPickerWorkflow(store, workingDir)
	return wf.createSession()
}

type sessionPickerWorkflow struct {
	store      sessionPickerStore
	workingDir string
}

func newSessionPickerWorkflow(store sessionPickerStore, workingDir string) *sessionPickerWorkflow {
	return &sessionPickerWorkflow{
		store:      store,
		workingDir: workingDir,
	}
}

func (w *sessionPickerWorkflow) prepareSelection() (domain.SessionListEvent, error) {
	summaries, err := w.store.List()
	if err != nil {
		return domain.SessionListEvent{}, err
	}

	// 1. Group sessions by WorkingDir
	groups := make(map[string][]domain.SessionMetadata)
	for _, s := range summaries {
		groups[s.WorkingDir] = append(groups[s.WorkingDir], s)
	}

	// 2. Sort sessions within each group by Updated descending
	for g := range groups {
		slices.SortFunc(groups[g], func(a, b domain.SessionMetadata) int {
			if a.Updated.After(b.Updated) {
				return -1
			}
			if a.Updated.Before(b.Updated) {
				return 1
			}
			return 0
		})
	}

	// 3. Collect other directory paths and sort alphabetically
	var otherDirs []string
	var currentDirExists bool
	for dir := range groups {
		if dir == w.workingDir {
			currentDirExists = true
		} else {
			otherDirs = append(otherDirs, dir)
		}
	}
	slices.Sort(otherDirs)

	// 4. Flatten groups: current directory first, then other folders sorted alphabetically
	var sortedSummaries []domain.SessionMetadata
	if currentDirExists {
		sortedSummaries = append(sortedSummaries, groups[w.workingDir]...)
	}
	for _, dir := range otherDirs {
		sortedSummaries = append(sortedSummaries, groups[dir]...)
	}

	// Active session is the one with Active=true in the current working directory
	var activeSessionID string
	if currentDirExists {
		for _, s := range groups[w.workingDir] {
			if s.Active {
				activeSessionID = s.ID
				break
			}
		}
	}

	return domain.SessionListEvent{
		Sessions:         sortedSummaries,
		CurrentSessionID: activeSessionID,
		WorkingDir:       w.workingDir,
	}, nil
}

func (w *sessionPickerWorkflow) applySelection(id string) (string, error) {
	meta, err := w.store.GetMetadata(id)
	if err != nil {
		return "", err
	}

	var targetDir string
	if meta.WorkingDir != "" && meta.WorkingDir != w.workingDir {
		targetDir = meta.WorkingDir
	}

	if err := w.store.SetActive(id, meta.WorkingDir); err != nil {
		return "", err
	}

	return targetDir, nil
}

func (w *sessionPickerWorkflow) createSession() (string, error) {
	existingBlank, err := w.store.FindBlank(w.workingDir)
	if err != nil {
		return "", err
	}
	if existingBlank != nil {
		if err := w.store.SetActive(existingBlank.ID, w.workingDir); err == nil {
			return existingBlank.ID, nil
		}
	}

	sess, err := w.store.Create(w.workingDir)
	if err != nil {
		return "", err
	}

	if err := w.store.SetActive(sess.ID, w.workingDir); err != nil {
		return "", err
	}

	return sess.ID, nil
}

func (w *sessionPickerWorkflow) renameSession(id, name string) error {
	return w.store.Rename(id, name)
}

func (w *sessionPickerWorkflow) deleteSession(id string) error {
	return w.store.Delete(id)
}
