package authui_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/state"
	authui "github.com/Cyclone1070/iav/internal/ui/auth"
	tea "github.com/charmbracelet/bubbletea"
)

type mockAuthManager struct {
	creds map[string]domain.Credential
}

func newMockAuthManager() *mockAuthManager {
	return &mockAuthManager{
		creds: make(map[string]domain.Credential),
	}
}

func (m *mockAuthManager) Set(id string, cred domain.Credential) error {
	m.creds[id] = cred
	return nil
}

func (m *mockAuthManager) Remove(id string) error {
	delete(m.creds, id)
	return nil
}

type mockRegistry struct {
	provider domain.Provider
	authMgr  *mockAuthManager
}

func (m *mockRegistry) ListProviders(ctx context.Context) ([]domain.ProviderInfo, error) {
	cred := (*domain.Credential)(nil)
	if c, ok := m.authMgr.creds[m.provider.ID()]; ok {
		cred = &c
	}
	return []domain.ProviderInfo{
		{ID: m.provider.ID(), Credential: cred},
	}, nil
}

func (m *mockRegistry) GetProvider(id string) (domain.Provider, bool) {
	if m.provider.ID() == id {
		return m.provider, true
	}
	return nil, false
}

func newTestState() *state.State {
	s := &state.State{}
	return s
}

type mockProvider struct {
	id string
}

func (m *mockProvider) ID() string { return m.id }
func (m *mockProvider) SupportedAuthMethods() []domain.AuthMethod {
	return []domain.AuthMethod{
		{
			ID:    "api_key",
			Label: "API Key",
			Fields: []domain.AuthField{
				{ID: "key", Label: "Key", Placeholder: "...", IsSecret: true},
			},
		},
	}
}
func (m *mockProvider) ListLLMs() []domain.LLMInfo {
	return nil
}
func (m *mockProvider) GetLLM(ctx context.Context, cred *domain.Credential, modelID string) (domain.LLM, error) {
	return nil, nil
}

type mockProviderMultiField struct {
	id string
}

func (m *mockProviderMultiField) ID() string { return m.id }
func (m *mockProviderMultiField) SupportedAuthMethods() []domain.AuthMethod {
	return []domain.AuthMethod{
		{
			ID:    "complex",
			Label: "Complex Auth",
			Fields: []domain.AuthField{
				{ID: "f1", Label: "F1", Placeholder: "F1"},
				{ID: "f2", Label: "F2", Placeholder: "F2"},
			},
		},
	}
}
func (m *mockProviderMultiField) ListLLMs() []domain.LLMInfo {
	return nil
}
func (m *mockProviderMultiField) GetLLM(ctx context.Context, cred *domain.Credential, modelID string) (domain.LLM, error) {
	return nil, nil
}

func TestInteractionFlowNoQuit(t *testing.T) {
	p := &mockProvider{id: "google"}
	authMgr := newMockAuthManager()
	registry := &mockRegistry{provider: p, authMgr: authMgr}
	model := authui.NewModel(registry, authMgr, newTestState())

	// 1. Initial State: Provider Selection
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	
	m, cmd := model.Update(msg)
	model = m.(*authui.Model)
	if isQuit(cmd) {
		t.Errorf("received tea.Quit after provider selection")
	}

	// 2. State: Method Selection
	m, cmd = model.Update(msg)
	model = m.(*authui.Model)
	if isQuit(cmd) {
		t.Errorf("received tea.Quit after method selection")
	}

	// 3. State: Field Collection - Empty input validation (Should QUIT now)
	m, cmd = model.Update(msg)
	model = m.(*authui.Model)
	if !isQuit(cmd) {
		t.Errorf("expected tea.Quit on invalid input")
	}

	// Submit actual input
	msgInput := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test-key")}
	m, _ = model.Update(msgInput)
	model = m.(*authui.Model)
	_, cmd = model.Update(msg)

	if !isQuit(cmd) {
		t.Errorf("expected tea.Quit after final field collection")
	}

	// Verify persistence!
	if _, ok := authMgr.creds["google"]; !ok {
		t.Errorf("expected auth data to be saved to mock manager")
	}
}

func TestMultiFieldInteraction(t *testing.T) {
	p := &mockProviderMultiField{id: "vertex"}
	authMgr := newMockAuthManager()
	registry := &mockRegistry{provider: p, authMgr: authMgr}
	model := authui.NewModel(registry, authMgr, newTestState())

	enter := tea.KeyMsg{Type: tea.KeyEnter}
	input := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

	// Select Provider
	m, _ := model.Update(enter)
	model = m.(*authui.Model)
	// Select Method
	m, _ = model.Update(enter)
	model = m.(*authui.Model)

	// Field 1
	m, _ = model.Update(input("val1"))
	model = m.(*authui.Model)
	_, cmd := model.Update(enter)
	if isQuit(cmd) {
		t.Fatal("quit prematurely after field 1")
	}

	// Field 2
	m, _ = model.Update(input("val2"))
	model = m.(*authui.Model)
	_, cmd = model.Update(enter)
	if !isQuit(cmd) {
		t.Fatal("expected quit after field 2")
	}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	return ok
}

func TestInteraction_BugFixes(t *testing.T) {
	p := &mockProvider{id: "google"}
	authMgr := newMockAuthManager()
	registry := &mockRegistry{provider: p, authMgr: authMgr}
	
	t.Run("Issue 1: q key should quit and clear screen", func(t *testing.T) {
		model := authui.NewModel(registry, authMgr, newTestState())
		m, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		if !isQuit(cmd) {
			t.Errorf("expected quit on 'q' key")
		}
		
		valModel := m.(*authui.Model)
		if valModel.View() != "" {
			t.Errorf("expected empty view on cancellation, got %q", valModel.View())
		}
	})

	t.Run("Issue 3: empty input validation", func(t *testing.T) {
		model := authui.NewModel(registry, authMgr, newTestState())
		// Advance to field collection
		m, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Select Provider
		model = m.(*authui.Model)
		m, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Select Method
		model = m.(*authui.Model)

		// Submit empty enter
		var cmd tea.Cmd
		m, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if !isQuit(cmd) {
			t.Errorf("expected quit on empty input")
		}
		
		valModel := m.(*authui.Model)
		if valModel.View() != "" {
			t.Errorf("expected empty view on error, got %q", valModel.View())
		}
	})

	t.Run("Issue 4: Panic on completion render", func(t *testing.T) {
		model := authui.NewModel(registry, authMgr, newTestState())
		
		// 1. Select provider
		m, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = m.(*authui.Model)
		// 2. Select method
		m, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = m.(*authui.Model)
		// 3. Enter input
		m, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("secret")})
		model = m.(*authui.Model)
		// 4. Submit final field
		m, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if !isQuit(cmd) {
			t.Fatal("expected quit after final submission")
		}
		
		// 5. Final Render (This is where Bubble Tea panics)
		// We use defer/recover to catch it and fail gracefully in the test
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("View() panicked: %v", r)
			}
		}()
		
		_ = m.View()
	})

	t.Run("Feature: Authorized status labels and deletion", func(t *testing.T) {
		p := &mockProvider{id: "google"}
		authMgr := newMockAuthManager()
		registry := &mockRegistry{provider: p, authMgr: authMgr}
		appState := &state.State{}
		appState.SetModel("google/gemini")
		
		// 1. Initially NOT authorized
		model := authui.NewModel(registry, authMgr, appState)
		view := model.View()
		if strings.Contains(strings.ToLower(view), "authorized") {
			t.Errorf("expected no authorized label initially")
		}

		// 2. Authorize
		authMgr.Set("google", domain.Credential{Type: "api_key", APIKey: "key"})
		
		// Refresh model
		model = authui.NewModel(registry, authMgr, appState)
		view = model.View()
		if !strings.Contains(strings.ToLower(view), "authorized") {
			t.Errorf("expected authorized label after setting creds")
		}

		// 3. Delete auth via 'd' key
		m, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
		model = m.(*authui.Model)
		
		// Verify credential removed from mock
		if _, ok := authMgr.creds["google"]; ok {
			t.Errorf("expected credential to be removed from auth manager")
		}

		// Verify state synced (model cleared)
		if appState.Model() != "" {
			t.Errorf("expected appState.Model() to be cleared after deleting auth")
		}

		// Verify label gone
		view = model.View()
		if strings.Contains(strings.ToLower(view), "authorized") {
			t.Errorf("expected authorized label to disappear after deletion")
		}
	})
}
