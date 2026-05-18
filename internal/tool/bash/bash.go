// Package bash provides tools for executing shell commands.
package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultWaitDuration = 10 * time.Second
	tailPreviewSize     = 2048
	streamReadChunkSize = 1024 * 1024
)

type commandExecutor interface {
	RunStreaming(ctx context.Context, command string, dir string, enableLogging bool) (*executor.StreamingCmd, error)
}

type pathResolver interface {
	Root() string
	DisplayPath(path string) string
}

type backgroundRegistrar interface {
	Register(id string, cmd *executor.StreamingCmd, logPath string, cancel context.CancelFunc, description, command string) error
}
type fileSystem interface {
	Open(path string) (domain.File, error)
	CreateAtomic(path string) (io.WriteCloser, error)
}

// Tool is a tool that allows executing shell commands with background task support.
type Tool struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
	taskManager     backgroundRegistrar
}

// NewTool creates a new Tool with injected dependencies.
func NewTool(fs fileSystem, commandExecutor commandExecutor, pathResolver pathResolver, taskManager backgroundRegistrar) *Tool {
	if fs == nil {
		panic("fs is required")
	}
	if commandExecutor == nil {
		panic("commandExecutor is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &Tool{
		fs:              fs,
		commandExecutor: commandExecutor,
		pathResolver:    pathResolver,
		taskManager:     taskManager,
	}
}

// Name returns the unique identifier for the bash tool.
func (t *Tool) Name() string {
	return "bash"
}

// IsConcurrentSafe indicates if the bash tool can be run concurrently.
func (t *Tool) IsConcurrentSafe() bool { return true }

func (t *Tool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "bash",
		Desc: `Execute a bash command on the local machine.

The working directory is always the workspace root (currently ` + fmt.Sprintf("\"%s\"", t.pathResolver.Root()) + `) for every command. Shell state does not persist between calls.

# Instructions
- If your command will create new directories or files, first use this tool to run "ls" to verify the parent directory exists and is the correct location.
- Always quote file paths that contain spaces with double quotes in your command (e.g., cd "path with spaces/file.txt").
- You may specify an optional timeout in milliseconds (default ` + fmt.Sprintf("%dms", defaultWaitDuration.Milliseconds()) + `). Commands exceeding this limit will be automatically moved to the background and you will receive a task ID.
- Use "run_in_background" to run a command in the background immediately without waiting.
- IMPORTANT: Background tasks (including those promoted on timeout) are tied to your current response turn. They will be terminated as soon as you stop talking and return control to the user. If you need a task (like a build or test) to complete before you finish, you MUST use the 'sleep' tool to wait for it.

## Parallel Execution
- If commands are independent and can run in parallel, make multiple bash tool calls in a single message for optimal performance.
- If commands depend on each other and must run sequentially, use a single bash call with "&&" to chain them together.
- Use ";" only when you need to run commands sequentially but don't care if earlier commands fail.

## Git Operations
- Only create commits when requested by the user. If unclear, ask first.
- Git Safety Protocol:
  * NEVER update the git config.
  * NEVER run destructive git commands (push --force, reset --hard, checkout ., restore ., clean -f, branch -D) unless the user explicitly requests these actions.
  * NEVER skip hooks (--no-verify, --no-gpg-sign, etc) unless the user explicitly requests it.
  * Always create NEW commits rather than amending, unless explicitly requested.
  * When staging files, prefer adding specific files by name rather than using "git add -A" or "git add .".
- In order to ensure good formatting, ALWAYS pass the commit message via a HEREDOC:
git commit -m "$(cat <<'EOF'
Commit message here.
EOF
)"

## Common Operations
- File search: Use the "glob" tool (NOT find or ls).
- Content search: Use the "grep" tool (NOT grep or rg).
- Read files: Use the "file_read" tool (NOT cat/head/tail).
- Edit files: Use the "file_edit" tool (NOT sed/awk).
- Write files: Use the "file_write" tool (NOT echo >/cat <<EOF).`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command": {
				Type:     schema.String,
				Desc:     fmt.Sprintf("The command to execute, with \"%s\" as workingDir", t.pathResolver.Root()),
				Required: true,
			},
			"description": {
				Type:     schema.String,
				Desc:     "Clear, concise description of what this command does in active voice.",
				Required: true,
			},
			"timeout": {
				Type: schema.Integer,
				Desc: fmt.Sprintf("Optional timeout in milliseconds (default %dms). Commands exceeding this will be automatically backgrounded.", defaultWaitDuration.Milliseconds()),
			},
			"run_in_background": {
				Type: schema.Boolean,
				Desc: "Set to true to run this command in the background. Note: The task will be killed when you finish your response unless you use 'sleep' to wait.",
			},
		}),
	}, nil
}

func (t *Tool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	req, err := t.validate(argumentsInJSON)
	if err != nil {
		return "", err
	}
	callID := compose.GetToolCallID(ctx)

	events, _ := runtimectx.EventSenderFrom(ctx)
	sink, _ := runtimectx.ToolDisplaySinkFrom(ctx)
	llmContent, finalDisplay := t.executeBash(ctx, req, events, callID)
	if events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: finalDisplay})
	}
	if sink != nil {
		sink(callID, finalDisplay)
	}
	return llmContent, nil
}

func (t *Tool) Preview(input *compose.ToolInput) domain.ToolDisplay {
	req := &Request{}
	if err := json.Unmarshal([]byte(input.Arguments), req); err != nil {
		return domain.NewStringDisplay(fmt.Sprintf("Run \"%s\"", t.Name()), "")
	}
	wdDisplay := t.pathResolver.DisplayPath(t.pathResolver.Root())
	return domain.NewBashDisplay(req.Description, req.Command, wdDisplay, "")
}

func (t *Tool) PreflightValidate(input *compose.ToolInput) error {
	_, err := t.validate(input.Arguments)
	return err
}

type Request struct {
	Command         string `json:"command"`
	Description     string `json:"description"`
	Timeout         int    `json:"timeout"`
	RunInBackground bool   `json:"run_in_background"`
}

type validatedRequest struct {
	wd              string
	wdDisplay       string
	command         string
	description     string
	timeoutMS       int
	runInBackground bool
}

func (t *Tool) validate(params string) (*validatedRequest, error) {
	var req Request
	if err := json.Unmarshal([]byte(params), &req); err != nil {
		return nil, fmt.Errorf("failed to parse bash parameters: %w", err)
	}

	// Validate command against block list
	if err := validateCommand(req.Command); err != nil {
		return nil, err
	}

	wd := t.pathResolver.Root()
	wdDisplay := t.pathResolver.DisplayPath(wd)
	if req.Description == "" {
		return nil, fmt.Errorf("description is required")
	}
	return &validatedRequest{
		wd:              wd,
		wdDisplay:       wdDisplay,
		command:         req.Command,
		description:     req.Description,
		timeoutMS:       req.Timeout,
		runInBackground: req.RunInBackground,
	}, nil
}

var forbiddenCommands = map[string]bool{
	"vim":    true,
	"vi":     true,
	"nvim":   true,
	"emacs":  true,
	"nano":   true,
	"pico":   true,
	"ed":     true,
	"less":   true,
	"more":   true,
	"most":   true,
	"top":    true,
	"htop":   true,
	"ssh":    true,
	"scp":    true,
	"telnet": true,
	"ftp":    true,
	"screen": true,
	"tmux":   true,
	"watch":  true,
}

var (
	operatorRegex = regexp.MustCompile(`^(&&|\|\||[&|;])$`)
	paddingRegex  = regexp.MustCompile(`(&&|\|\||[&|;])`)
)

func validateCommand(cmd string) error {
	// Inject spaces around operators to handle attached commands like &&vim
	padded := paddingRegex.ReplaceAllString(cmd, " $1 ")
	fields := strings.Fields(padded)
	if len(fields) == 0 {
		return nil
	}

	nextIsBaseCommand := true
	for _, token := range fields {
		if nextIsBaseCommand {
			// Extract basename to handle /usr/bin/vim
			base := filepath.Base(token)
			if forbiddenCommands[base] {
				return fmt.Errorf("command %q is interactive or forbidden for security reasons", base)
			}
			nextIsBaseCommand = false
			continue
		}

		// If this token is an operator, the next one will be a base command
		if operatorRegex.MatchString(token) {
			nextIsBaseCommand = true
		}
	}

	return nil
}

func (t *Tool) tryPromoteToBackground(req *validatedRequest, streamCmd *executor.StreamingCmd, taskCancel context.CancelFunc) (llmContent string, display domain.BashDisplay, ok bool) {
	if t.taskManager == nil {
		return "", domain.BashDisplay{}, false
	}

	streamCmd.DisableAutoCleanup()

	id := streamCmd.ID()
	if err := t.taskManager.Register(id, streamCmd, streamCmd.LogPath(), taskCancel, req.description, req.command); err != nil {
		return "", domain.BashDisplay{}, false
	}

	display = domain.NewBashDisplay(req.description, req.command, req.wdDisplay, fmt.Sprintf("(Command ran in the background. Live output is saved at \"%s\")", streamCmd.LogPath()))
	llmContent = fmt.Sprintf("command ran in the background. Live output is saved at \"%s\", use \"read_file\" tool to read it, \"task_list\" to check status, \"task_stop\" to terminate, \"sleep\" to wait for task to finish. Command will be terminated if you do not call any tool and end the ReAct loop.\n\n<background_task_id>%s</background_task_id>\n<cwd>%s</cwd>", streamCmd.LogPath(), id, req.wd)
	return llmContent, display, true
}

func (t *Tool) executeBash(ctx context.Context, req *validatedRequest, events runtimectx.EventSender, callID string) (string, domain.ToolDisplay) {
	// We pass the OG context down so executor has SessionID and responds to App exit,
	// but we don't wrap it with timeout so the process survives backgrounding.
	taskCtx, taskCancel := context.WithCancel(ctx)

	promoted := false
	defer func() {
		if !promoted {
			taskCancel()
		}
	}()

	// Map LLM timeout to waitDuration (default 10s)
	waitDuration := defaultWaitDuration
	if req.timeoutMS > 0 {
		waitDuration = time.Duration(req.timeoutMS) * time.Millisecond
	}

	streamCmd, err := t.commandExecutor.RunStreaming(taskCtx, req.command, req.wd, true)
	if err != nil {
		d := domain.NewBashDisplay(req.description, req.command, req.wdDisplay, "")
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("failed to run command: %v\n\n<cwd>%s</cwd>", err, req.wd), d
	}

	var wg sync.WaitGroup
	if events != nil {
		rd := streamCmd.Output()
		wg.Go(func() {
			buf := make([]byte, streamReadChunkSize)
			for {
				n, readErr := rd.Read(buf)
				if n > 0 {
					events.SendUIUpdate(domain.ToolStreamEvent{CallID: callID, Chunk: string(buf[:n])})
				}
				if readErr != nil {
					return
				}
			}
		})
		defer func() {
			stopStreamReader(rd)
			wg.Wait()
		}()
	}

	if req.runInBackground {
		if llmContent, d, ok := t.tryPromoteToBackground(req, streamCmd, taskCancel); ok {
			promoted = true
			return llmContent, d
		}
	}

	done := make(chan struct{})
	var res *executor.Result
	var waitErr error

	go func() {
		res, waitErr = streamCmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Done case handled below
	case <-time.After(waitDuration):
		if llmContent, d, ok := t.tryPromoteToBackground(req, streamCmd, taskCancel); ok {
			promoted = true
			return llmContent, d
		}
		<-done
	}

	d := domain.NewBashDisplay(req.description, req.command, req.wdDisplay, "")

	llmOutput := res.Stdout
	uiOutput := res.Stdout
	if res.LogPath != "" {
		preview := t.readTail(res.LogPath, tailPreviewSize)
		llmOutput = fmt.Sprintf("Output too large. Full output saved to %s. Use `read_file` tool to read full output.\n\nPreview output (last 2KB):\n%s", res.LogPath, preview)
		uiOutput = fmt.Sprintf("(Output too large, saved to %s)", res.LogPath)
	}

	if waitErr != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		}
		d.Error = domain.ToolErrorFailed
		llmOutput = fmt.Sprintf("Error: command failed: %v\n\n%s", waitErr, llmOutput)
	}

	d.CapturedOutput = uiOutput
	return fmt.Sprintf("%s\n\n<exit_code>%d</exit_code>\n<cwd>%s</cwd>", llmOutput, res.ExitCode, req.wd), d
}

func (t *Tool) readTail(path string, size int64) string {
	f, err := t.fs.Open(path)
	if err != nil {
		return ""
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			slog.Error("failed to close file", "error", closeErr)
		}
	}()

	info, err := f.Stat()
	if err != nil {
		return ""
	}

	totalSize := info.Size()
	offset := max(totalSize-size, 0)

	_, err = f.Seek(offset, io.SeekStart)
	if err != nil {
		return ""
	}

	buf, err := io.ReadAll(f)
	if err != nil {
		return ""
	}

	return string(buf)
}

func stopStreamReader(r io.Reader) {
	if closer, ok := r.(io.Closer); ok {
		_ = closer.Close()
	}
	if stopper, ok := r.(interface{ Stop() }); ok {
		stopper.Stop()
	}
}
