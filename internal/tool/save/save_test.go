package save

import (
	"context"
	"testing"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/runtimectx"
	"github.com/cloudwego/eino/compose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStore struct {
	saved   map[string]*domain.SavedCommand
	lastErr error
}

func (m *mockStore) Get(name string) (*domain.SavedCommand, bool) {
	if m.saved == nil {
		return nil, false
	}
	cmd, ok := m.saved[name]
	return cmd, ok
}

func (m *mockStore) Save(name, command, description string) error {
	if m.lastErr != nil {
		return m.lastErr
	}
	if m.saved == nil {
		m.saved = make(map[string]*domain.SavedCommand)
	}
	m.saved[name] = &domain.SavedCommand{
		Name: name, Command: command, Description: description,
	}
	return nil
}

func (m *mockStore) seed(name, command string) {
	if m.saved == nil {
		m.saved = make(map[string]*domain.SavedCommand)
	}
	m.saved[name] = &domain.SavedCommand{Name: name, Command: command}
}

type capturingBus struct {
	updates []domain.UIUpdate
}

func (b *capturingBus) SendUIUpdate(u domain.UIUpdate) { b.updates = append(b.updates, u) }

func TestTool_Info(t *testing.T) {
	tool := NewTool(&mockStore{})

	def, err := tool.Info(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, def)
	assert.Equal(t, "save_command", def.Name)
	assert.Contains(t, def.Desc, "Saves a bash command")
}

func TestTool_Execute_SavesNewCommand(t *testing.T) {
	tool := NewTool(&mockStore{})
	p := &toolParams{Name: "git imp", Command: "git status --porcelain", Description: "Quick git status"}

	llmContent, display := tool.execute(context.Background(), p)

	assert.Contains(t, llmContent, "git imp")
	assert.Contains(t, llmContent, "Saved")
	sd := display.(domain.StringDisplay)
	assert.Equal(t, "", sd.Error)
	assert.Contains(t, sd.Description, "git imp")
}

func TestTool_Execute_UpdatesExistingWithOverride(t *testing.T) {
	ms := &mockStore{}
	ms.seed("git imp", "old command")
	tool := NewTool(ms)
	p := &toolParams{Name: "git imp", Command: "new command", Override: true}

	llmContent, display := tool.execute(context.Background(), p)

	assert.Contains(t, llmContent, "Updated")
	sd := display.(domain.StringDisplay)
	assert.Equal(t, "", sd.Error)
}

func TestTool_Execute_ErrorsOnExistingWithoutOverride(t *testing.T) {
	ms := &mockStore{}
	ms.seed("git imp", "old command")
	tool := NewTool(ms)
	p := &toolParams{Name: "git imp", Command: "new command"}

	llmContent, display := tool.execute(context.Background(), p)

	assert.Contains(t, llmContent, "already exists")
	sd := display.(domain.StringDisplay)
	assert.Equal(t, domain.ToolErrorFailed, sd.Error)
}

func TestTool_Execute_NewNameWithoutOverrideSaves(t *testing.T) {
	ms := &mockStore{}
	ms.seed("other", "echo existing")
	tool := NewTool(ms)
	p := &toolParams{Name: "new", Command: "echo hello"}

	llmContent, display := tool.execute(context.Background(), p)

	assert.Contains(t, llmContent, "Saved")
	sd := display.(domain.StringDisplay)
	assert.Equal(t, "", sd.Error)
}

func TestTool_Execute_StoreError(t *testing.T) {
	ms := &mockStore{lastErr: assert.AnError}
	tool := NewTool(ms)
	p := &toolParams{Name: "test", Command: "echo hi"}

	llmContent, display := tool.execute(context.Background(), p)

	assert.Contains(t, llmContent, "Error")
	sd := display.(domain.StringDisplay)
	assert.Equal(t, domain.ToolErrorFailed, sd.Error)
}

func TestTool_Execute_ContextCancelled(t *testing.T) {
	tool := NewTool(&mockStore{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := &toolParams{Name: "test", Command: "echo hi"}
	llmContent, display := tool.execute(ctx, p)

	assert.Equal(t, domain.ToolErrorCancelled, llmContent)
	sd := display.(domain.StringDisplay)
	assert.Equal(t, domain.ToolErrorCancelled, sd.Error)
}

func TestTool_Validate_EmptyName(t *testing.T) {
	tool := NewTool(&mockStore{})

	_, err := tool.validate(`{"name": "", "command": "echo hi"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name must not be empty")
}

func TestTool_Validate_WhitespaceName(t *testing.T) {
	tool := NewTool(&mockStore{})

	_, err := tool.validate(`{"name": "   ", "command": "echo hi"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name must not be empty")
}

func TestTool_Validate_EmptyCommand(t *testing.T) {
	tool := NewTool(&mockStore{})

	_, err := tool.validate(`{"name": "test", "command": ""}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command must not be empty")
}

func TestTool_Preview(t *testing.T) {
	tool := NewTool(&mockStore{})

	input := &compose.ToolInput{Arguments: `{"name": "git imp", "command": "git status"}`}
	display := tool.Preview(input)
	sd, ok := display.(domain.StringDisplay)
	require.True(t, ok, "Preview should return StringDisplay")
	assert.Contains(t, sd.Description, "git imp")
	assert.Contains(t, sd.Content, "git status")
}

func TestTool_Preview_InvalidJSON(t *testing.T) {
	tool := NewTool(&mockStore{})

	input := &compose.ToolInput{Arguments: `not json`}
	display := tool.Preview(input)
	sd, ok := display.(domain.StringDisplay)
	require.True(t, ok)
	assert.Contains(t, sd.Description, "save_command")
}

func TestTool_Preview_WithOverride(t *testing.T) {
	tool := NewTool(&mockStore{})

	input := &compose.ToolInput{Arguments: `{"name": "git imp", "command": "git status", "override": true}`}
	display := tool.Preview(input)
	sd, ok := display.(domain.StringDisplay)
	require.True(t, ok)
	assert.Contains(t, sd.Description, "git imp")
}

func TestTool_PreflightValidate_Valid(t *testing.T) {
	tool := NewTool(&mockStore{})

	input := &compose.ToolInput{Arguments: `{"name": "test", "command": "echo ok"}`}
	err := tool.PreflightValidate(input)
	assert.NoError(t, err)
}

func TestTool_PreflightValidate_Invalid(t *testing.T) {
	tool := NewTool(&mockStore{})

	input := &compose.ToolInput{Arguments: `{"name": "", "command": "echo ok"}`}
	err := tool.PreflightValidate(input)
	assert.Error(t, err)
}

func TestTool_IsConcurrentSafe(t *testing.T) {
	tool := NewTool(&mockStore{})
	assert.True(t, tool.IsConcurrentSafe())
}

func TestTool_InvokableRun_EmitsToolEndEvent(t *testing.T) {
	bus := &capturingBus{}
	ctx := runtimectx.WithEventSender(context.Background(), bus)

	tool := NewTool(&mockStore{})
	_, err := tool.InvokableRun(ctx, `{"name":"test","command":"echo hi"}`)
	require.NoError(t, err)
	require.Len(t, bus.updates, 1)
	end, ok := bus.updates[0].(domain.ToolEndEvent)
	require.True(t, ok)
	sd, ok := end.Display.(domain.StringDisplay)
	require.True(t, ok)
	assert.Equal(t, "", sd.Error)
}
