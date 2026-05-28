package workflow

import (
	"context"
	"slices"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
)

type sessionPickerStore interface {
	List() ([]domain.SessionSummary, error)
	Get(id string) (*domain.Session, error)
	Save(sess *domain.Session) error
	Create() (*domain.Session, error)
	FindBlank() (*domain.SessionSummary, error)
	Rename(id, name string) error
	Delete(id string) error
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

// PickerResult represents the result of running the session picker.
type PickerResult struct {
	SwitchCwd string
	Created   bool
	Err       error
}

// RunSessionPicker starts the session management workflow asynchronously.
func RunSessionPicker(ctx context.Context, deps *SessionPickerDeps) <-chan PickerResult {
	done := make(chan PickerResult, 1)
	go func() {
		defer close(done)
		wf := newSessionPickerWorkflow(deps.Store, deps.WorkingDir)

		// 1. Send initial snapshot
		snapshot, err := wf.prepareSelection()
		if err != nil {
			done <- PickerResult{Err: err}
			return
		}
		deps.Bus.SendUIUpdate(snapshot)

		// 2. Loop for actions
		for {
			select {
			case <-ctx.Done():
				done <- PickerResult{Err: ctx.Err()}
				return
			case act, ok := <-deps.Bus.WorkflowActions():
				if !ok {
					done <- PickerResult{}
					return
				}

				switch a := act.(type) {
				case domain.SelectSessionAction:
					targetDir, err := wf.applySelection(a.ID)
					if err != nil {
						done <- PickerResult{Err: err}
						return
					}
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- PickerResult{SwitchCwd: targetCwdClean(targetDir)}
					return

				case domain.CreateSessionAction:
					id, err := wf.createSession()
					if err != nil {
						done <- PickerResult{Err: err}
						return
					}
					// If the new session is successfully created, we don't switch cwd
					_ = id
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- PickerResult{}
					return

				case domain.RenameSessionAction:
					if err := wf.renameSession(a.ID, a.Name); err != nil {
						done <- PickerResult{Err: err}
						return
					}

				case domain.DeleteSessionAction:
					if err := wf.deleteSession(a.ID); err != nil {
						done <- PickerResult{Err: err}
						return
					}

				case domain.StopAction:
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- PickerResult{Err: context.Canceled}
					return
				}

				// After any mutation (or if we need to refresh), send a new snapshot
				snapshot, err := wf.prepareSelection()
				if err != nil {
					done <- PickerResult{Err: err}
					return
				}
				deps.Bus.SendUIUpdate(snapshot)
			}
		}
	}()
	return done
}

func targetCwdClean(dir string) string {
	return dir
}

// CreateSession is a public helper to create a new session and update state.
// Used by the 'iav session new' command.
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
	groups := make(map[string][]domain.SessionSummary)
	for _, s := range summaries {
		groups[s.WorkingDir] = append(groups[s.WorkingDir], s)
	}

	// 2. Sort sessions within each group by Updated descending
	for g := range groups {
		slices.SortFunc(groups[g], func(a, b domain.SessionSummary) int {
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
	var sortedSummaries []domain.SessionSummary
	if currentDirExists {
		sortedSummaries = append(sortedSummaries, groups[w.workingDir]...)
	}
	for _, dir := range otherDirs {
		sortedSummaries = append(sortedSummaries, groups[dir]...)
	}

	// Active session is the latest session in the current working directory
	var activeSessionID string
	if currentDirExists && len(groups[w.workingDir]) > 0 {
		activeSessionID = groups[w.workingDir][0].ID
	}

	return domain.SessionListEvent{
		Sessions:         sortedSummaries,
		CurrentSessionID: activeSessionID,
		WorkingDir:       w.workingDir,
	}, nil
}

func (w *sessionPickerWorkflow) applySelection(id string) (string, error) {
	sess, err := w.store.Get(id)
	if err != nil {
		return "", err
	}

	var targetDir string
	if sess.WorkingDir == "" {
		sess.WorkingDir = w.workingDir
	} else if sess.WorkingDir != w.workingDir {
		targetDir = sess.WorkingDir
	}

	sess.Updated = time.Now()
	if err := w.store.Save(sess); err != nil {
		return "", err
	}

	return targetDir, nil
}

func (w *sessionPickerWorkflow) createSession() (string, error) {
	existingBlank, err := w.store.FindBlank()
	if err != nil {
		return "", err
	}
	// Blank sessions with no directory or matching directory can be reused
	if existingBlank != nil && (existingBlank.WorkingDir == "" || existingBlank.WorkingDir == w.workingDir) {
		sess, err := w.store.Get(existingBlank.ID)
		if err == nil {
			sess.WorkingDir = w.workingDir
			sess.Updated = time.Now()
			if err := w.store.Save(sess); err == nil {
				return sess.ID, nil
			}
		}
	}

	sess, err := w.store.Create()
	if err != nil {
		return "", err
	}

	sess.WorkingDir = w.workingDir
	if err := w.store.Save(sess); err != nil {
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
