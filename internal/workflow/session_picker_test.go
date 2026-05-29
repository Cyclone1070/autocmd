package workflow

import (
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockSessionPickerStore struct {
	mock.Mock
}

func (m *mockSessionPickerStore) List() ([]domain.SessionMetadata, error) {
	args := m.Called()
	return args.Get(0).([]domain.SessionMetadata), args.Error(1)
}

func (m *mockSessionPickerStore) GetMetadata(id string) (*domain.SessionMetadata, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SessionMetadata), args.Error(1)
}

func (m *mockSessionPickerStore) Create(workingDir string) (*domain.Session, error) {
	args := m.Called(workingDir)
	return args.Get(0).(*domain.Session), args.Error(1)
}

func (m *mockSessionPickerStore) FindBlank(workingDir string) (*domain.SessionMetadata, error) {
	args := m.Called(workingDir)
	if v := args.Get(0); v != nil {
		return v.(*domain.SessionMetadata), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSessionPickerStore) Rename(id, name string) error {
	args := m.Called(id, name)
	return args.Error(0)
}

func (m *mockSessionPickerStore) Delete(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *mockSessionPickerStore) SetActive(id, workingDir string) error {
	args := m.Called(id, workingDir)
	return args.Error(0)
}

type mockSessionPickerBus struct {
	mock.Mock
}

func (m *mockSessionPickerBus) SendUIUpdate(update domain.UIUpdate) {
	m.Called(update)
}

func (m *mockSessionPickerBus) WorkflowActions() <-chan domain.Action {
	args := m.Called()
	return args.Get(0).(<-chan domain.Action)
}

func TestSessionPickerWorkflow(t *testing.T) {
	now := time.Now()
	summaries := []domain.SessionMetadata{
		{ID: "s1", Name: "S1", WorkingDir: "/dirB", Updated: now.Add(-10 * time.Minute)},
		{ID: "s2", Name: "S2", WorkingDir: "/dirA", Updated: now.Add(-5 * time.Minute)},
		{ID: "s3", Name: "S3", WorkingDir: "/current", Updated: now.Add(-2 * time.Minute), Active: true},
		{ID: "s4", Name: "S4", WorkingDir: "/current", Updated: now},
		{ID: "s5", Name: "S5", WorkingDir: "", Updated: now.Add(-20 * time.Minute)},
	}

	t.Run("prepareSelection groups and sorts correctly", func(t *testing.T) {
		store := new(mockSessionPickerStore)
		store.On("List").Return(summaries, nil)

		wf := newSessionPickerWorkflow(store, "/current")
		res, err := wf.prepareSelection()

		assert.NoError(t, err)
		require.Len(t, res.Sessions, 5)

		// Assert group sorting:
		// 1. Current directory (/current) first, sorted by updated desc: s4 (newest), then s3 (active)
		assert.Equal(t, "s4", res.Sessions[0].ID)
		assert.Equal(t, "s3", res.Sessions[1].ID)

		// 2. Global ("") group, then other folders sorted alphabetically: "" is global, then /dirA, then /dirB
		// Depending on how "" is sorted, if sorted alphabetically: "" (global) comes before "/"
		// So groups: "" -> "/dirA" -> "/dirB"
		assert.Equal(t, "s5", res.Sessions[2].ID) // WorkingDir: ""
		assert.Equal(t, "s2", res.Sessions[3].ID) // WorkingDir: "/dirA"
		assert.Equal(t, "s1", res.Sessions[4].ID) // WorkingDir: "/dirB"

		// Active session should be the one with Active=true in current folder
		assert.Equal(t, "s3", res.CurrentSessionID)
		assert.Equal(t, "/current", res.WorkingDir)
	})

	t.Run("applySelection touches and saves session", func(t *testing.T) {
		store := new(mockSessionPickerStore)
		meta := &domain.SessionMetadata{ID: "s1", WorkingDir: "/dirB", Updated: now.Add(-10 * time.Minute)}
		store.On("GetMetadata", "s1").Return(meta, nil)
		store.On("SetActive", "s1", "/current").Return(nil)

		wf := newSessionPickerWorkflow(store, "/current")
		targetCwd, err := wf.applySelection("s1")

		assert.NoError(t, err)
		assert.Equal(t, "/dirB", targetCwd)
		store.AssertExpectations(t)
	})

	t.Run("applySelection scopes global session to current folder", func(t *testing.T) {
		store := new(mockSessionPickerStore)
		meta := &domain.SessionMetadata{ID: "s5", WorkingDir: "", Updated: now.Add(-20 * time.Minute)}
		store.On("GetMetadata", "s5").Return(meta, nil)
		store.On("SetActive", "s5", "/current").Return(nil)

		wf := newSessionPickerWorkflow(store, "/current")
		targetCwd, err := wf.applySelection("s5")

		assert.NoError(t, err)
		assert.Equal(t, "", targetCwd)
		store.AssertExpectations(t)
	})
}
