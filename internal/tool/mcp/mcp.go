package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/Cyclone1070/autocmd/internal/domain"

	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// ServerConfig defines how to run an MCP server.
type ServerConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     []string          `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Config represents the mcp.json format.
type Config struct {
	McpServers map[string]ServerConfig `json:"mcpServers"`
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
			return &Config{McpServers: make(map[string]ServerConfig)}, nil
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

type serverResult struct {
	name  string
	tools []einotool.BaseTool
	cli   client.MCPClient
	err   error
}

// Start spawns all configured MCP servers in parallel, initializes them,
// and returns Eino tools.
func (m *Manager) Start(ctx context.Context) ([]einotool.BaseTool, error) {
	if len(m.cfg.McpServers) == 0 {
		return nil, nil
	}

	results := make(chan serverResult, len(m.cfg.McpServers))
	var wg sync.WaitGroup

	for name, serverCfg := range m.cfg.McpServers {
		wg.Add(1)
		go func(name string, serverCfg ServerConfig) {
			defer wg.Done()

			var cli client.MCPClient
			var err error

			if serverCfg.URL != "" {
				if m.remoteCreator == nil {
					results <- serverResult{name: name, err: fmt.Errorf("no remote client creator configured for remote server %s", name)}
					return
				}
				cli, err = m.remoteCreator(ctx, serverCfg.URL, serverCfg.Headers)
			} else {
				if m.stdioCreator == nil {
					results <- serverResult{name: name, err: fmt.Errorf("no stdio client creator configured for server %s", name)}
					return
				}
				cli, err = m.stdioCreator(serverCfg.Command, serverCfg.Env, serverCfg.Args...)
			}

			if err != nil {
				results <- serverResult{name: name, err: fmt.Errorf("failed to start mcp server %s: %w", name, err)}
				return
			}

			// Start client connection if required (e.g. for SSE clients)
			if starter, ok := cli.(interface{ Start(context.Context) error }); ok {
				if err := starter.Start(ctx); err != nil {
					cli.Close()
					results <- serverResult{name: name, err: fmt.Errorf("failed to start connection for mcp server %s: %w", name, err)}
					return
				}
			}

			// Handshake/Initialize
			initReq := mcp.InitializeRequest{}
			initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
			initReq.Params.ClientInfo = mcp.Implementation{
				Name:    "autocmd",
				Version: "1.0.0",
			}
			_, err = cli.Initialize(ctx, initReq)
			if err != nil {
				cli.Close()
				results <- serverResult{name: name, err: fmt.Errorf("failed to initialize mcp server %s: %w", name, err)}
				return
			}

			// Retrieve tools via Eino MCP component
			tools, err := mcpp.GetTools(ctx, &mcpp.Config{Cli: cli})
			if err != nil {
				cli.Close()
				results <- serverResult{name: name, err: fmt.Errorf("failed to get tools from mcp server %s: %w", name, err)}
				return
			}

			results <- serverResult{name: name, tools: tools, cli: cli}
		}(name, serverCfg)
	}

	wg.Wait()
	close(results)

	var allClients []client.MCPClient
	var allTools []einotool.BaseTool
	var firstErr error

	for res := range results {
		if res.err != nil {
			if firstErr == nil {
				firstErr = res.err
			}
			if res.cli != nil {
				res.cli.Close()
			}
			continue
		}
		allClients = append(allClients, res.cli)
		allTools = append(allTools, res.tools...)
	}

	if firstErr != nil {
		for _, cli := range allClients {
			cli.Close()
		}
		return nil, firstErr
	}

	m.clients = allClients
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
