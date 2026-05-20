package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Cyclone1070/iav/internal/domain"

	einotool "github.com/cloudwego/eino/components/tool"
	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// McpServerConfig defines how to run an MCP server.
type McpServerConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     []string          `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Config represents the mcp.json format.
type Config struct {
	McpServers map[string]McpServerConfig `json:"mcpServers"`
}

// ParseConfig parses the mcp.json contents.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ResolveConfigPath returns the default path to the mcp.json file.
func ResolveConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, domain.ConfigBaseDir, domain.AppName, "mcp.json"), nil
}

// LoadConfigPath loads the config from the given file path.
func LoadConfigPath(path string) (*Config, error) {
	// nosec G304 -- Config file path is resolved dynamically or specified via trusted user flags
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{McpServers: make(map[string]McpServerConfig)}, nil
		}
		return nil, err
	}
	return ParseConfig(data)
}



// StdioCreator defines a function type for creating stdio MCPClient.
type StdioCreator func(command string, env []string, args ...string) (client.MCPClient, error)

// RemoteCreator defines a function type for creating remote (SSE or HTTP) MCPClient.
type RemoteCreator func(ctx context.Context, url string, headers map[string]string) (client.MCPClient, error)

// DefaultStdioCreator is the standard creator for Stdio clients.
func DefaultStdioCreator(command string, env []string, args ...string) (client.MCPClient, error) {
	return client.NewStdioMCPClient(command, env, args...)
}

// DefaultRemoteCreator is the standard creator for remote (SSE or Streamable HTTP) clients.
func DefaultRemoteCreator(ctx context.Context, url string, headers map[string]string) (client.MCPClient, error) {
	useStreamable := false
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err == nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusMethodNotAllowed {
				useStreamable = true
			}
		}
	}

	if useStreamable {
		var streamOpts []transport.StreamableHTTPCOption
		if len(headers) > 0 {
			streamOpts = append(streamOpts, transport.WithHTTPHeaders(headers))
		}
		return client.NewStreamableHttpClient(url, streamOpts...)
	}

	var opts []transport.ClientOption
	if len(headers) > 0 {
		opts = append(opts, client.WithHeaders(headers))
	}
	return client.NewSSEMCPClient(url, opts...)
}

// Manager manages MCP server clients and lifecycles.
type Manager struct {
	cfg           *Config
	stdioCreator  StdioCreator
	remoteCreator RemoteCreator
	clients       []client.MCPClient
}

// NewManager creates a new Manager.
func NewManager(cfg *Config, stdioCreator StdioCreator, remoteCreator RemoteCreator) *Manager {
	if stdioCreator == nil {
		stdioCreator = DefaultStdioCreator
	}
	if remoteCreator == nil {
		remoteCreator = DefaultRemoteCreator
	}
	return &Manager{
		cfg:           cfg,
		stdioCreator:  stdioCreator,
		remoteCreator: remoteCreator,
	}
}

// Start spawns all configured MCP servers, initializes them, and returns Eino tools.
func (m *Manager) Start(ctx context.Context) ([]einotool.BaseTool, error) {
	var allTools []einotool.BaseTool

	for name, serverCfg := range m.cfg.McpServers {
		var cli client.MCPClient
		var err error

		if serverCfg.URL != "" {
			if m.remoteCreator == nil {
				_ = m.Close()
				return nil, fmt.Errorf("no remote client creator configured for remote server %s", name)
			}
			cli, err = m.remoteCreator(ctx, serverCfg.URL, serverCfg.Headers)
		} else {
			if m.stdioCreator == nil {
				_ = m.Close()
				return nil, fmt.Errorf("no stdio client creator configured for server %s", name)
			}
			cli, err = m.stdioCreator(serverCfg.Command, serverCfg.Env, serverCfg.Args...)
		}

		if err != nil {
			_ = m.Close() // Clean up any clients started so far
			return nil, fmt.Errorf("failed to start mcp server %s: %w", name, err)
		}
		m.clients = append(m.clients, cli)

		// Start client connection if required (e.g. for SSE clients)
		if starter, ok := cli.(interface{ Start(context.Context) error }); ok {
			if err := starter.Start(ctx); err != nil {
				_ = m.Close()
				return nil, fmt.Errorf("failed to start connection for mcp server %s: %w", name, err)
			}
		}

		// Handshake/Initialize
		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{
			Name:    "iav",
			Version: "1.0.0",
		}
		_, err = cli.Initialize(ctx, initReq)
		if err != nil {
			_ = m.Close()
			return nil, fmt.Errorf("failed to initialize mcp server %s: %w", name, err)
		}

		// Retrieve tools via Eino MCP component
		tools, err := mcpp.GetTools(ctx, &mcpp.Config{
			Cli: cli,
		})
		if err != nil {
			_ = m.Close()
			return nil, fmt.Errorf("failed to get tools from mcp server %s: %w", name, err)
		}

		allTools = append(allTools, tools...)
	}

	return allTools, nil
}

// Close terminates all active client connections.
func (m *Manager) Close() error {
	var firstErr error
	for _, cli := range m.clients {
		if err := cli.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	m.clients = nil
	return firstErr
}
