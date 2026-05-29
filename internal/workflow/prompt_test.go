package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockSessionStore struct {
	mock.Mock
}

func (m *mockSessionStore) Create() (*domain.Session, error) {
	args := m.Called()
	return args.Get(0).(*domain.Session), args.Error(1)
}

func (m *mockSessionStore) GetSession(id string) (*domain.Session, error) {
	args := m.Called(id)
	return args.Get(0).(*domain.Session), args.Error(1)
}

func (m *mockSessionStore) SaveSession(s *domain.Session) error {
	args := m.Called(s)
	return args.Error(0)
}

func (m *mockSessionStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	args := m.Called(ctx, llm, target)
	return args.String(0), args.Error(1)
}

type mockWorkspaceSessionStore struct {
	mock.Mock
}

func (m *mockWorkspaceSessionStore) FindActiveForDir(dir string) (*domain.SessionMetadata, error) {
	args := m.Called(dir)
	if v := args.Get(0); v != nil {
		return v.(*domain.SessionMetadata), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockWorkspaceSessionStore) Create() (*domain.Session, error) {
	args := m.Called()
	return args.Get(0).(*domain.Session), args.Error(1)
}

func (m *mockWorkspaceSessionStore) GetSession(id string) (*domain.Session, error) {
	args := m.Called(id)
	return args.Get(0).(*domain.Session), args.Error(1)
}

func (m *mockWorkspaceSessionStore) SaveSession(s *domain.Session) error {
	args := m.Called(s)
	return args.Error(0)
}

type mockLLM struct {
	mock.Mock
}

func (m *mockLLM) ID() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockLLM) DisplayName() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockLLM) ComputeTokens(ctx context.Context, messages []*schema.Message) (int, error) {
	args := m.Called(ctx, messages)
	return args.Int(0), args.Error(1)
}

func (m *mockLLM) Model() model.ToolCallingChatModel {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(model.ToolCallingChatModel)
}

func (m *mockLLM) ContextWindow() int {
	args := m.Called()
	return args.Int(0)
}

type mockAgent struct {
	mock.Mock
}

func (m *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	args := m.Called(ctx, sess, input)
	return args.Error(0)
}

type mockActionForwarder struct {
	mock.Mock
}

func (m *mockActionForwarder) Deliver(act domain.Action) {
	m.Called(act)
}

func TestResolveWorkspaceSession(t *testing.T) {
	t.Run("Finds active session", func(t *testing.T) {
		store := new(mockWorkspaceSessionStore)
		summary := &domain.SessionMetadata{ID: "sess-123", WorkingDir: "/dir", Active: true}
		sess := &domain.Session{SessionMetadata: domain.SessionMetadata{ID: "sess-123", WorkingDir: "/dir", Active: true}}

		store.On("FindActiveForDir", "/dir").Return(summary, nil)
		store.On("GetSession", "sess-123").Return(sess, nil)

		res, err := ResolveWorkspaceSession(store, "/dir")
		assert.NoError(t, err)
		assert.Equal(t, "sess-123", res.ID)
		store.AssertExpectations(t)
	})

	t.Run("Creates new session if none exists", func(t *testing.T) {
		store := new(mockWorkspaceSessionStore)
		sess := &domain.Session{SessionMetadata: domain.SessionMetadata{ID: "sess-new"}}

		store.On("FindActiveForDir", "/dir").Return((*domain.SessionMetadata)(nil), nil)
		store.On("Create").Return(sess, nil)
		store.On("SaveSession", mock.Anything).Return(nil)

		res, err := ResolveWorkspaceSession(store, "/dir")
		assert.NoError(t, err)
		assert.Equal(t, "sess-new", res.ID)
		assert.Equal(t, "/dir", res.WorkingDir)
		store.AssertExpectations(t)
	})


}

func TestRunPrompt_ActionForwarding(t *testing.T) {
	ctx := t.Context()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)
	bus := eventbus.New()
	forwarder := new(mockActionForwarder)

	sess := &domain.Session{SessionMetadata: domain.SessionMetadata{ID: "id"}}
	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		Agent:        agent,
		Bus:          bus,
		Forwarder:    forwarder,
		Session:      sess,
	}

	store.On("SaveSession", mock.Anything).Return(nil)
	store.On("GenerateName", mock.Anything, mock.Anything, mock.Anything).Return("Name", nil)

	// Keep the agent running so we can send an action
	agentRunDone := make(chan struct{})
	agent.On("Run", mock.Anything, mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		<-agentRunDone
	}).Return(nil)

	RunPrompt(ctx, "hello", deps)

	// Send an action that should be forwarded
	act := domain.QuestionAnswerAction{CallID: "call-1"}
	forwarder.On("Deliver", act).Return()

	bus.SendAction(act)

	// Give the goroutine time to process
	time.Sleep(50 * time.Millisecond)

	forwarder.AssertExpectations(t)
	close(agentRunDone)
}

func TestRunPrompt_GREEN(t *testing.T) {
	ctx := t.Context()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)
	bus := eventbus.New()

	sess := &domain.Session{SessionMetadata: domain.SessionMetadata{ID: "sess-123"}}
	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		Agent:        agent,
		Bus:          bus,
		Session:      sess,
	}

	store.On("SaveSession", mock.Anything).Return(nil)
	store.On("GenerateName", mock.Anything, mock.Anything, "hello").Return("New Session", nil)

	agent.On("Run", mock.MatchedBy(func(ctx context.Context) bool {
		id, ok := domain.GetSessionID(ctx)
		return ok && id == "sess-123"
	}), mock.Anything, "hello").Return(nil)

	done := RunPrompt(ctx, "hello", deps)
	err := <-done

	assert.NoError(t, err)
	store.AssertExpectations(t)
	llm.AssertExpectations(t)
}

func TestRunPrompt_ExistingNamedSession_DoesNotHang(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)
	bus := eventbus.New()

	sess := &domain.Session{SessionMetadata: domain.SessionMetadata{ID: "existing-id", Name: "Existing Session"}}

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		Agent:        agent,
		Bus:          bus,
		Session:      sess,
	}

	store.On("SaveSession", mock.Anything).Return(nil)
	agent.On("Run", mock.Anything, sess, "hello").Return(nil)

	done := RunPrompt(ctx, "hello", deps)

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("RunPrompt did not complete for existing named session")
	}

	// Verify no WaitingForNamingEvent was sent because session already had a name
	var receivedWaiting bool
	doneEventReceived := false
	for !doneEventReceived {
		select {
		case ev := <-bus.UIUpdates():
			switch ev.(type) {
			case domain.WaitingForNamingEvent:
				receivedWaiting = true
			case domain.DoneEvent:
				doneEventReceived = true
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timed out waiting for DoneEvent")
		}
	}
	assert.False(t, receivedWaiting, "Should not receive WaitingForNamingEvent when session already named")
}

func TestRunPrompt_DoesNotCloseBus(t *testing.T) {
	ctx := t.Context()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)

	eb := eventbus.New()
	type trackableBus struct {
		*eventbus.EventBus
		closed bool
	}
	bus := &trackableBus{EventBus: eb}

	sess := &domain.Session{SessionMetadata: domain.SessionMetadata{ID: "id"}}
	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		Agent:        agent,
		Bus:          bus.EventBus,
		Session:      sess,
	}

	store.On("SaveSession", mock.Anything).Return(nil)
	store.On("GenerateName", mock.Anything, mock.Anything, mock.Anything).Return("New Session", nil)
	agent.On("Run", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	done := RunPrompt(ctx, "hello", deps)
	err := <-done
	assert.NoError(t, err)

	assert.False(t, bus.closed, "RunPrompt should NOT close the bus")
}

func TestRunPrompt_NamingRace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)
	bus := eventbus.New()

	sess := &domain.Session{SessionMetadata: domain.SessionMetadata{ID: "race-id"}}
	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		Agent:        agent,
		Bus:          bus,
		Session:      sess,
	}

	store.On("SaveSession", mock.Anything).Return(nil)

	// Sync channels to force interleaving
	agentStartedAppending := make(chan struct{})

	// Simulate GenerateName waiting for agent to start appending
	store.On("GenerateName", mock.Anything, mock.Anything, "hello").
		Run(func(_ mock.Arguments) {
			<-agentStartedAppending
		}).
		Return("Named Session", nil)

	// Simulate Agent.Run appending messages and then signaling
	agent.On("Run", mock.Anything, mock.Anything, "hello").
		Run(func(args mock.Arguments) {
			s := args.Get(1).(*domain.Session)
			for i := range 10 {
				s.Messages = append(s.Messages, &schema.Message{Role: schema.User, Content: "msg"})
				if i == 5 {
					close(agentStartedAppending)
				}
			}
		}).
		Return(nil)

	done := RunPrompt(ctx, "hello", deps)

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}
}

func TestRunPrompt_EmitsIndicators(t *testing.T) {
	ctx := t.Context()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)
	bus := eventbus.New()

	sess := &domain.Session{SessionMetadata: domain.SessionMetadata{ID: "id"}}
	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		Agent:        agent,
		Bus:          bus,
		Session:      sess,
	}

	store.On("SaveSession", mock.Anything).Return(nil)
	store.On("GenerateName", mock.Anything, mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		time.Sleep(50 * time.Millisecond)
	}).Return("Name", nil)

	agentRunStarted := make(chan struct{})
	agent.On("Run", mock.Anything, mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		close(agentRunStarted)
		time.Sleep(10 * time.Millisecond)
	}).Return(nil)

	done := RunPrompt(ctx, "hello", deps)

	select {
	case ev := <-bus.UIUpdates():
		_, ok := ev.(domain.ConnectingEvent)
		assert.True(t, ok)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ConnectingEvent timeout")
	}

	<-agentRunStarted

	select {
	case ev := <-bus.UIUpdates():
		_, ok := ev.(domain.WaitingForNamingEvent)
		assert.True(t, ok)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("WaitingForNamingEvent timeout")
	}

	select {
	case ev := <-bus.UIUpdates():
		_, ok := ev.(domain.DoneEvent)
		assert.True(t, ok)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("DoneEvent timeout")
	}

	<-done
}
