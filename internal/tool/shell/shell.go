package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

// ShellTool executes commands on the local machine.
type ShellTool struct {
	commandExecutor commandExecutor
	pathResolver    pathResolver
}

// NewShellTool creates a new ShellTool with injected dependencies.
func NewShellTool(commandExecutor commandExecutor, pathResolver pathResolver) *ShellTool {
	if commandExecutor == nil {
		panic("commandExecutor is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &ShellTool{
		commandExecutor: commandExecutor,
		pathResolver:    pathResolver,
	}
}

// Name returns the tool's identifier.
func (t *ShellTool) Name() string {
	return "shell"
}

func (t *ShellTool) IsConcurrentSafe() bool { return true }

// Definition returns the tool's schema for the LLM using eino schema.
func (t *ShellTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "shell",
		Desc: "Execute a shell command on the local machine. Runs in the workspace root using the current process environment. Use shell builtins (e.g. timeout) if you need a time limit.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command": {
				Type: schema.Array,
				Desc: "The command to execute, including arguments.",
				ElemInfo: &schema.ParameterInfo{
					Type: schema.String,
				},
				Required: true,
			},
			"comment": {
				Type:     schema.String,
				Desc:     "A brief comment (under 80 characters) describing the purpose of the command for display in the UI. Mandatory.",
				Required: true,
			},
		}),
	}
}

// Prepare validates the request and returns an Invocation.
// Note: we intentionally do not start the streaming process here so cancellation
// can be driven solely by Execute's ctx.
func (t *ShellTool) Prepare(params string) (domain.Invocation, error) {
	var req struct {
		Command []string `json:"command"`
		Comment string   `json:"comment"`
	}
	if err := json.Unmarshal([]byte(params), &req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if len(req.Command) == 0 {
		return nil, fmt.Errorf("command is required")
	}
	if strings.TrimSpace(req.Comment) == "" {
		return nil, fmt.Errorf("comment is required")
	}

	wd := t.pathResolver.Root()
	if wd == "" {
		return nil, fmt.Errorf("workspace root not set")
	}

	env := os.Environ()

	return &shellInvocation{
		commandExecutor: t.commandExecutor,
		wd:              wd,
		env:             env,
		command:        req.Command,
		commandStr:     strings.Join(req.Command, " "),
		comment:        req.Comment,
	}, nil
}

type shellInvocation struct {
	commandExecutor commandExecutor
	wd              string
	env             []string
	command        []string
	commandStr     string
	comment        string

	pipeOnce   sync.Once
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
}

func (i *shellInvocation) Stream() io.Reader {
	i.pipeOnce.Do(func() {
		i.pipeReader, i.pipeWriter = io.Pipe()
	})
	return i.pipeReader
}

func (i *shellInvocation) Display() domain.ToolDisplay {
	return domain.NewShellDisplay(i.comment, i.commandStr, "")
}

func (i *shellInvocation) cancelledDisplay() domain.ShellDisplay {
	sh := domain.NewShellDisplay(i.comment, i.commandStr, "")
	sh.Error = domain.ToolErrorCancelled
	return sh
}

func (i *shellInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay, error) {
	if ctx.Err() != nil {
		if i.pipeWriter != nil {
			_ = i.pipeWriter.CloseWithError(ctx.Err())
		}
		return "execution cancelled", i.cancelledDisplay(), ctx.Err()
	}

	streamCmd, err := i.commandExecutor.RunStreaming(ctx, i.command, i.wd, i.env)
	if err != nil {
		if i.pipeWriter != nil {
			_ = i.pipeWriter.CloseWithError(err)
		}
		sh := domain.NewShellDisplay(i.comment, i.commandStr, "")
		sh.Error = err.Error()
		return fmt.Sprintf("Error: %v", err), sh, errors.New("Execution failed")
	}

	// Pump streamed stdout/stderr to the pipe so the UI can consume chunks.
	if i.pipeWriter != nil {
		go func() {
			_, _ = io.Copy(i.pipeWriter, streamCmd.Output())
			_ = i.pipeWriter.Close()
		}()
	}

	result, err := streamCmd.Wait()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			if i.pipeWriter != nil {
				_ = i.pipeWriter.CloseWithError(err)
			}
			return "", i.cancelledDisplay(), err
		}
		sh := domain.NewShellDisplay(i.comment, i.commandStr, "")
		sh.Error = err.Error()
		return fmt.Sprintf("Error: %v", err), sh, errors.New("Execution failed")
	}

	output := result.Stdout
	exitCode := result.ExitCode

	var truncationNote string
	if result.Truncated {
		truncationNote = "\n(Output truncated)"
	}

	sh := domain.NewShellDisplay(i.comment, i.commandStr, output)
	return fmt.Sprintf("%s\n\n(Exit code: %d)%s", output, exitCode, truncationNote), sh, nil
}
