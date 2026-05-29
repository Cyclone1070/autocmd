package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	configJSON := `{
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
				"env": ["FOO=bar"]
			}
		}
	}`

	cfg, err := ParseConfig([]byte(configJSON))
	assert.NoError(t, err)
	assert.Len(t, cfg.McpServers, 1)

	fsServer, exists := cfg.McpServers["filesystem"]
	assert.True(t, exists)
	assert.Equal(t, "npx", fsServer.Command)
	assert.Equal(t, []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"}, fsServer.Args)
	assert.Equal(t, []string{"FOO=bar"}, fsServer.Env)
}

func TestLoadConfig_EmptyOrInvalid(t *testing.T) {
	_, err := ParseConfig([]byte(`{invalid`))
	assert.Error(t, err)
}

func TestResolveConfigPath(t *testing.T) {
	path, err := ResolveConfigPath()
	assert.NoError(t, err)
	expectedSuffix := filepath.Join(domain.ConfigBaseDir, domain.AppName, "mcp.json")
	assert.Contains(t, path, expectedSuffix)
}

func TestLoadConfigPath_NonExistent(t *testing.T) {
	cfg, err := LoadConfigPath("/nonexistent-path-12345/mcp.json")
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Empty(t, cfg.McpServers)
}

type mockMCPClient struct {
	initialized bool
	closed      bool
	tools       []mcp.Tool
}

func (m *mockMCPClient) Initialize(ctx context.Context, request mcp.InitializeRequest) (*mcp.InitializeResult, error) {
	m.initialized = true
	return &mcp.InitializeResult{
		ServerInfo: mcp.Implementation{Name: "mock-server", Version: "1.0.0"},
	}, nil
}

func (m *mockMCPClient) ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	return &mcp.ListToolsResult{Tools: m.tools}, nil
}

func (m *mockMCPClient) Close() error {
	m.closed = true
	return nil
}

// Implement other methods of client.MCPClient interface as no-ops.
func (m *mockMCPClient) Ping(ctx context.Context) error { return nil }

func (m *mockMCPClient) ListResourcesByPage(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	return nil, nil
}

func (m *mockMCPClient) ListResources(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	return nil, nil
}

func (m *mockMCPClient) ListResourceTemplatesByPage(ctx context.Context, request mcp.ListResourceTemplatesRequest) (*mcp.ListResourceTemplatesResult, error) {
	return nil, nil
}

func (m *mockMCPClient) ListResourceTemplates(ctx context.Context, request mcp.ListResourceTemplatesRequest) (*mcp.ListResourceTemplatesResult, error) {
	return nil, nil
}

func (m *mockMCPClient) ReadResource(ctx context.Context, request mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return nil, nil
}

func (m *mockMCPClient) Subscribe(ctx context.Context, request mcp.SubscribeRequest) error {
	return nil
}

func (m *mockMCPClient) Unsubscribe(ctx context.Context, request mcp.UnsubscribeRequest) error {
	return nil
}

func (m *mockMCPClient) ListPromptsByPage(ctx context.Context, request mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error) {
	return nil, nil
}

func (m *mockMCPClient) ListPrompts(ctx context.Context, request mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error) {
	return nil, nil
}

func (m *mockMCPClient) GetPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return nil, nil
}

func (m *mockMCPClient) ListToolsByPage(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	return nil, nil
}

func (m *mockMCPClient) CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return nil, nil
}
func (m *mockMCPClient) SetLevel(ctx context.Context, request mcp.SetLevelRequest) error { return nil }
func (m *mockMCPClient) Complete(ctx context.Context, request mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	return nil, nil
}
func (m *mockMCPClient) OnNotification(handler func(notification mcp.JSONRPCNotification)) {}

func TestManager_Lifecycle(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		McpServers: map[string]ServerConfig{
			"test-server": {
				Command: "node",
				Args:    []string{"server.js"},
				Env:     []string{"ENV_VAR=1"},
			},
		},
	}

	mockCli := &mockMCPClient{
		tools: []mcp.Tool{
			{
				Name:        "get_weather",
				Description: "Gets the weather",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
				},
			},
		},
	}

	// Injected creator to mock the client creation
	creator := func(command string, env []string, args ...string) (client.MCPClient, error) {
		assert.Equal(t, "node", command)
		assert.Equal(t, []string{"ENV_VAR=1"}, env)
		assert.Equal(t, []string{"server.js"}, args)
		return mockCli, nil
	}

	mgr := NewManager(cfg, creator, nil)
	tools, err := mgr.Start(ctx)
	assert.NoError(t, err)
	assert.Len(t, tools, 1)

	info, err := tools[0].Info(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "get_weather", info.Name)

	assert.True(t, mockCli.initialized)
	assert.False(t, mockCli.closed)

	err = mgr.Close()
	assert.NoError(t, err)
	assert.True(t, mockCli.closed)
}

func TestManager_SSERemoteLifecycle(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		McpServers: map[string]ServerConfig{
			"remote-server": {
				URL: "http://localhost:8080/sse",
			},
		},
	}

	mockCli := &mockMCPClient{
		tools: []mcp.Tool{
			{
				Name:        "remote_tool",
				Description: "Gets remote data",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
				},
			},
		},
	}

	calledSSE := false
	remoteCreator := func(ctx context.Context, url string, headers map[string]string) (client.MCPClient, error) {
		assert.Equal(t, "http://localhost:8080/sse", url)
		calledSSE = true
		return mockCli, nil
	}

	// We'll update NewManager to accept both creators or a custom creator options struct.
	// Let's pass nil for stdioCreator in this test.
	mgr := NewManager(cfg, nil, remoteCreator)
	tools, err := mgr.Start(ctx)
	assert.NoError(t, err)
	assert.Len(t, tools, 1)

	info, err := tools[0].Info(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "remote_tool", info.Name)
	assert.True(t, calledSSE)

	err = mgr.Close()
	assert.NoError(t, err)
	assert.True(t, mockCli.closed)
}

func TestManager_StartError(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		McpServers: map[string]ServerConfig{
			"error-server": {
				Command: "bad-cmd",
			},
		},
	}

	creator := func(command string, env []string, args ...string) (client.MCPClient, error) {
		return nil, errors.New("failed to start process")
	}

	mgr := NewManager(cfg, creator, nil)
	_, err := mgr.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start process")
}

func TestManager_RemoteHeaders(t *testing.T) {
	ctx := context.Background()
	cfgJSON := `{
		"mcpServers": {
			"remote-headers-server": {
				"url": "http://localhost:8080/sse",
				"headers": {
					"Authorization": "Bearer token123"
				}
			}
		}
	}`
	cfg, err := ParseConfig([]byte(cfgJSON))
	assert.NoError(t, err)

	mockCli := &mockMCPClient{
		tools: []mcp.Tool{
			{
				Name: "test_tool",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
				},
			},
		},
	}
	calledRemote := false

	remoteCreator := func(ctx context.Context, url string, headers map[string]string) (client.MCPClient, error) {
		assert.Equal(t, "http://localhost:8080/sse", url)
		assert.Equal(t, map[string]string{"Authorization": "Bearer token123"}, headers)
		calledRemote = true
		return mockCli, nil
	}

	mgr := NewManager(cfg, nil, remoteCreator)
	_, err = mgr.Start(ctx)
	assert.NoError(t, err)
	assert.True(t, calledRemote)
}
