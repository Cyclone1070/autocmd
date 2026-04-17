package bash

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultWaitDuration = 10 * time.Second
)

type commandExecutor interface {
	RunStreaming(ctx context.Context, command string, dir string, enableLogging bool) (*executor.StreamingCmd, error)
}

type pathResolver interface {
	Root() string
}

type backgroundRegistrar interface {
	Register(id string, cmd *executor.StreamingCmd, logPath string, cancel context.CancelFunc, description, command string) error
}
type fileSystem interface {
	Open(path string) (domain.File, error)
	CreateAtomic(path string) (io.WriteCloser, error)
}

type BashTool struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
	taskManager     backgroundRegistrar
}

// NewBashTool creates a new BashTool with injected dependencies.
func NewBashTool(fs fileSystem, commandExecutor commandExecutor, pathResolver pathResolver, taskManager backgroundRegistrar) *BashTool {
	if fs == nil {
		panic("fs is required")
	}
	if commandExecutor == nil {
		panic("commandExecutor is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &BashTool{
		fs:              fs,
		commandExecutor: commandExecutor,
		pathResolver:    pathResolver,
		taskManager:     taskManager,
	}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) IsConcurrentSafe() bool { return true }

func (t *BashTool) Definition() *schema.ToolInfo {
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
			"comment": {
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
	}
}

func (t *BashTool) Prepare(params string) (domain.Invocation, error) {
	var req struct {
		Command         string `json:"command"`
		Comment         string `json:"comment"`
		Timeout         int    `json:"timeout"`
		RunInBackground bool   `json:"run_in_background"`
	}
	if err := json.Unmarshal([]byte(params), &req); err != nil {
		return nil, fmt.Errorf("failed to parse bash parameters: %v", err)
	}

	// Validate command against block list
	if err := validateCommand(req.Command); err != nil {
		return nil, err
	}

	wd := t.pathResolver.Root()
	if req.Comment == "" {
		return nil, fmt.Errorf("comment is required")
	}
	return &bashInvocation{
		fs:              t.fs,
		commandExecutor: t.commandExecutor,
		taskManager:     t.taskManager,
		wd:              wd,
		command:         req.Command,
		comment:         req.Comment,
		timeoutMS:       req.Timeout,
		runInBackground: req.RunInBackground,
		proxy:           newProxyReader(),
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

type bashInvocation struct {
	fs              fileSystem
	commandExecutor commandExecutor
	taskManager     backgroundRegistrar
	wd              string
	command         string
	comment         string
	timeoutMS       int
	runInBackground bool
	proxy           *proxyReader
}

func (i *bashInvocation) Display() domain.ToolDisplay {
	return domain.NewBashDisplay(i.comment, i.command, "")
}

func (i *bashInvocation) Stream() io.Reader {
	return i.proxy
}

func (i *bashInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	// We pass the OG context down so executor has SessionID and responds to App exit,
	// but we don't wrap it with timeout so the process survives backgrounding.
	taskCtx, taskCancel := context.WithCancel(ctx)

	promoted := false
	defer func() {
		if !promoted {
			taskCancel()
		}
		i.proxy.Close()
	}()

	// Map LLM timeout to waitDuration (default 10s)
	waitDuration := defaultWaitDuration
	if i.timeoutMS > 0 {
		waitDuration = time.Duration(i.timeoutMS) * time.Millisecond
	}

	streamCmd, err := i.commandExecutor.RunStreaming(taskCtx, i.command, i.wd, true)
	if err != nil {
		d := domain.NewBashDisplay(i.comment, i.command, "")
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("failed to run command: %v", err), d
	}

	// Plug the real follower into the proxy
	i.proxy.Set(streamCmd.Output())

	if i.runInBackground {
		if i.taskManager != nil {
			id := streamCmd.ID()
			if err := i.taskManager.Register(id, streamCmd, streamCmd.LogPath(), taskCancel, i.comment, i.command); err == nil {
				promoted = true
				d := domain.NewBashDisplay(i.comment, i.command, "(command running in background)")
				return fmt.Sprintf("the command is running in the background. Live output is saved at \"%s\", use \"read_file\" tool to read it.\n\n<background_task_id>%s</background_task_id>", streamCmd.LogPath(), id), d
			}
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
		if i.taskManager != nil {
			id := streamCmd.ID()
			if err := i.taskManager.Register(id, streamCmd, streamCmd.LogPath(), taskCancel, i.comment, i.command); err == nil {
				promoted = true
				d := domain.NewBashDisplay(i.comment, i.command, "(command running in background)")
				return fmt.Sprintf("the command is running in the background. Live output is saved at \"%s\", use \"read_file\" tool to read it.\n\n<background_task_id>%s</background_task_id>", streamCmd.LogPath(), id), d
			}
		}
		<-done
	}

	d := domain.NewBashDisplay(i.comment, i.command, "")

	llmOutput := res.Stdout
	uiOutput := res.Stdout
	if res.LogPath != "" {
		preview := i.readTail(res.LogPath, 2048)
		llmOutput = fmt.Sprintf("Output too large. Full output saved to %s. Use `read_file` tool to read full output.\n\nPreview output (last 2KB):\n%s", res.LogPath, preview)
		uiOutput = fmt.Sprintf("(Output too large, saved to %s)", res.LogPath)
	}

	if waitErr != nil {
		if errors.Is(waitErr, context.DeadlineExceeded) {
			d.Error = domain.ToolErrorTimedOut
			llmOutput = fmt.Sprintf("%s\n\n<execution_status>\n  <exit_code>%d</exit_code>\n  <timedout>true</timedout>\n</execution_status>", llmOutput, res.ExitCode)
		} else if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		} else {
			d.Error = domain.ToolErrorFailed
			d.CapturedOutput = uiOutput
			return fmt.Sprintf("Error: command failed: %v\n\n%s", waitErr, llmOutput), d
		}
	}

	d.CapturedOutput = uiOutput
	return llmOutput, d
}

func (i *bashInvocation) readTail(path string, size int64) string {
	f, err := i.fs.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

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

type proxyReader struct {
	r     io.Reader
	mu    sync.Mutex
	ch    chan io.Reader
	close chan struct{}
	once  sync.Once
}

func newProxyReader() *proxyReader {
	return &proxyReader{
		ch:    make(chan io.Reader, 1),
		close: make(chan struct{}),
	}
}

func (p *proxyReader) Set(r io.Reader) {
	select {
	case <-p.close:
		// Already closed, terminate the incoming reader immediately
		if closer, ok := r.(io.Closer); ok {
			_ = closer.Close()
		}
		if stopper, ok := r.(interface{ Stop() }); ok {
			stopper.Stop()
		}
	case p.ch <- r:
	default:
	}
}

func (p *proxyReader) Close() {
	p.once.Do(func() {
		close(p.close)
		p.mu.Lock()
		r := p.r
		p.mu.Unlock()
		if r != nil {
			if closer, ok := r.(io.Closer); ok {
				_ = closer.Close()
			}
			if stopper, ok := r.(interface{ Stop() }); ok {
				stopper.Stop()
			}
		}
	})
}

func (p *proxyReader) Read(b []byte) (int, error) {
	p.mu.Lock()
	r := p.r
	p.mu.Unlock()

	if r == nil {
		select {
		case newR := <-p.ch:
			// We got a reader, but Close() may have already fired.
			// Check if close was signalled while we were waiting.
			select {
			case <-p.close:
				// Close() already ran but couldn't Stop() because p.r was nil.
				// We must stop the reader ourselves.
				if stopper, ok := newR.(interface{ Stop() }); ok {
					stopper.Stop()
				}
				return 0, io.EOF
			default:
			}
			p.mu.Lock()
			p.r = newR
			r = newR
			p.mu.Unlock()
		case <-p.close:
			return 0, io.EOF
		}
	}
	return r.Read(b)
}
