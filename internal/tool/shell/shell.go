package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
)

// ShellTool executes commands on the local machine.
type ShellTool struct {
	envFileOps          envFileOps
	commandExecutor     commandExecutor
	defaultShellTimeout time.Duration
	pathResolver        pathResolver
}

// NewShellTool creates a new ShellTool with injected dependencies.
func NewShellTool(
	envFileOps envFileOps,
	commandExecutor commandExecutor,
	defaultShellTimeout time.Duration,
	pathResolver pathResolver,
) *ShellTool {
	if envFileOps == nil {
		panic("envFileOps is required")
	}
	if commandExecutor == nil {
		panic("commandExecutor is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &ShellTool{
		envFileOps:          envFileOps,
		commandExecutor:     commandExecutor,
		defaultShellTimeout: defaultShellTimeout,
		pathResolver:        pathResolver,
	}
}

// Name returns the tool's identifier.
func (t *ShellTool) Name() string {
	return "shell"
}

// Declaration returns the tool's schema for the LLM.
func (t *ShellTool) Declaration() domain.Declaration {
	return domain.Declaration{
		Name:        "shell",
		Description: "Execute a shell command on the local machine.",
		Parameters: &domain.Schema{
			Type: domain.TypeObject,
			Properties: map[string]*domain.Schema{
				"command": {
					Type:        domain.TypeArray,
					Description: "The command to execute, including arguments.",
					Items: &domain.Schema{
						Type: domain.TypeString,
					},
				},
				"working_dir": {
					Type:        domain.TypeString,
					Description: "Working directory for execution. Defaults to workspace root.",
				},
				"timeout_seconds": {
					Type:        domain.TypeInteger,
					Description: "Timeout in seconds. Defaults to configuration.",
				},
				"env": {
					Type:        domain.TypeObject,
					Description: "Environment variables to set.",
				},
				"env_files": {
					Type:        domain.TypeArray,
					Description: "Paths to .env files to load.",
					Items: &domain.Schema{
						Type: domain.TypeString,
					},
				},
				"comment": {
					Type:        domain.TypeString,
					Description: "A brief comment describing the purpose of the command for display purposes.",
				},
			},
			Required: []string{"command", "comment"},
		},
	}
}

// Prepare validates the request, resolves paths, and starts the streaming command.
func (t *ShellTool) Prepare(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
	// 1. Parse Parameters
	var req struct {
		Command        []string          `json:"command"`
		WorkingDir     string            `json:"working_dir"`
		TimeoutSeconds int               `json:"timeout_seconds"`
		Env            map[string]string `json:"env"`
		EnvFiles       []string          `json:"env_files"`
		Comment        string            `json:"comment"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	// 2. Validate
	if len(req.Command) == 0 {
		return nil, fmt.Errorf("command is required")
	}

	// 3. Resolve Working Directory
	workingDir := req.WorkingDir
	if workingDir == "" {
		workingDir = "."
	}
	wdAbs, err := t.pathResolver.Abs(workingDir)
	if err != nil {
		return nil, err
	}

	// 4. Prepare Environment
	env := os.Environ()
	for _, envFile := range req.EnvFiles {
		envFilePath, err := t.pathResolver.Abs(envFile)
		if err != nil {
			return nil, err
		}
		envVars, err := ParseEnvFile(t.envFileOps, envFilePath)
		if err != nil {
			return nil, err
		}
		for k, v := range envVars {
			env = append(env, k+"="+v)
		}
	}
	for k, v := range req.Env {
		env = append(env, k+"="+v)
	}

	// 5. Calculate Timeout
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if req.TimeoutSeconds <= 0 {
		timeout = t.defaultShellTimeout
	}

	// 6. Start Streaming Command via Executor
	streamCmd, err := t.commandExecutor.RunStreaming(ctx, req.Command, wdAbs, env, timeout)
	if err != nil {
		return nil, err
	}

	empty := ""
	return &shellInvocation{
		streamCmd:      streamCmd,
		commandStr:     strings.Join(req.Command, " "),
		comment:        req.Comment,
		capturedOutput: &empty,
	}, nil
}

type shellInvocation struct {
	streamCmd      *executor.StreamingCmd
	commandStr     string
	comment        string
	capturedOutput *string
}

func (i *shellInvocation) Display() domain.ToolDisplay {
	return domain.NewShellDisplay(
		i.comment,
		i.commandStr,
		i.streamCmd.Output(),
		i.capturedOutput,
	)
}

func (i *shellInvocation) Execute(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// Wait for command to complete and get result
	result, err := i.streamCmd.Wait()

	// Handle infrastructure errors
	if err != nil {
		if errors.Is(err, executor.ErrTimeout) {
			return fmt.Sprintf("Error: %s\n(Command timed out)", result.Stdout), executor.ErrTimeout
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		return fmt.Sprintf("Error: %v", err), err
	}

	// Format output
	output := result.Stdout
	exitCode := result.ExitCode

	// Populate captured output for history
	if i.capturedOutput != nil {
		*i.capturedOutput = output
	}

	// Add truncation note if applicable
	var truncationNote string
	if result.Truncated {
		truncationNote = "\n(Output truncated)"
	}

	return fmt.Sprintf("%s\n\n(Exit code: %d)%s", output, exitCode, truncationNote), nil
}
