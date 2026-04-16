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

// BashTool executes commands on the local machine.
type BashTool struct {
	fs                fileSystem
	commandExecutor   commandExecutor
	pathResolver      pathResolver
	taskManager       backgroundRegistrar
	foregroundTimeout time.Duration
}

// NewBashTool creates a new BashTool with injected dependencies.
func NewBashTool(fs fileSystem, commandExecutor commandExecutor, pathResolver pathResolver, taskManager backgroundRegistrar, foregroundTimeout time.Duration) *BashTool {
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
		fs:                fs,
		commandExecutor:   commandExecutor,
		pathResolver:      pathResolver,
		taskManager:       taskManager,
		foregroundTimeout: foregroundTimeout,
	}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) IsConcurrentSafe() bool { return true }

func (t *BashTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "bash",
		Desc: "Execute a bash command on the local machine.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command": {
				Type:     schema.String,
				Desc:     fmt.Sprintf("The command to execute, with \"%s\" as workingDir", t.pathResolver.Root()),
				Required: true,
			},
			"comment": {
				Type: schema.String,
				Desc: `Clear, concise description of what this command does in active voice. Never use words like "complex" or "risk" in the description - just describe what it does.

For simple commands (git, npm, standard CLI tools), keep it brief (5-10 words):
- ls → "List files in current directory"
- git status → "Show working tree status"
- npm install → "Install package dependencies"

For commands that are harder to parse at a glance (piped commands, obscure flags, etc.), add enough context to clarify what it does:
- find . -name "*.tmp" -exec rm {} \; → "Find and delete all .tmp files recursively"
- git reset --hard origin/main → "Discard all local changes and match remote main"
- curl -s url | jq '.data[]' → "Fetch JSON from URL and extract data array elements"`,
				Required: true,
			},
			"timeout": {
				Type: schema.Integer,
				Desc: "Optional timeout in milliseconds",
			},
			"run_in_background": {
				Type: schema.Boolean,
				Desc: "Set to true to run this command in the background.",
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
		fs:                t.fs,
		commandExecutor:   t.commandExecutor,
		taskManager:       t.taskManager,
		wd:                wd,
		command:           req.Command,
		commandStr:        req.Command,
		comment:           req.Comment,
		foregroundTimeout: t.foregroundTimeout,
		timeoutMS:         req.Timeout,
		runInBackground:   req.RunInBackground,
		proxy:             newProxyReader(),
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
	fs                fileSystem
	commandExecutor   commandExecutor
	taskManager       backgroundRegistrar
	wd                string
	command           string
	commandStr        string
	comment           string
	foregroundTimeout time.Duration
	timeoutMS         int
	runInBackground   bool
	proxy             *proxyReader
}

func (i *bashInvocation) Display() domain.ToolDisplay {
	return domain.NewBashDisplay(i.comment, i.commandStr, "")
}

func (i *bashInvocation) Stream() io.Reader {
	return i.proxy
}

func (i *bashInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	if i.timeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(i.timeoutMS)*time.Millisecond)
		defer cancel()
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())
	// Ensure we cancel it if we don't end up in background or if things fail.
	promoted := false
	defer func() {
		if !promoted {
			bgCancel()
		}
	}()

	streamCmd, err := i.commandExecutor.RunStreaming(bgCtx, i.command, i.wd, true)
	if err != nil {
		i.proxy.Close()
		d := domain.NewBashDisplay(i.comment, i.commandStr, "")
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("failed to run command: %v", err), d
	}

	// Plug the real follower into the proxy
	i.proxy.Set(streamCmd.Output())

	if i.runInBackground {
		if i.taskManager != nil {
			id := streamCmd.ID()
			if err := i.taskManager.Register(id, streamCmd, streamCmd.LogPath(), bgCancel, i.comment, i.commandStr); err == nil {
				promoted = true
				d := domain.NewBashDisplay(i.comment, i.commandStr, "(command running in background)")
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
	case <-time.After(i.foregroundTimeout):
		if i.taskManager != nil {
			id := streamCmd.ID()
			if err := i.taskManager.Register(id, streamCmd, streamCmd.LogPath(), bgCancel, i.comment, i.commandStr); err == nil {
				promoted = true
				d := domain.NewBashDisplay(i.comment, i.commandStr, "(command running in background)")
				return fmt.Sprintf("the command is running in the background. Live output is saved at \"%s\", use \"read_file\" tool to read it.\n\n<background_task_id>%s</background_task_id>", streamCmd.LogPath(), id), d
			}
		}
		<-done
	case <-ctx.Done():
		// Outer context (timeout or manual interruption) cancelled.
		// If we haven't promoted to background, bgCancel will be called by defer.
		<-done
	}

	i.proxy.Close()
	d := domain.NewBashDisplay(i.comment, i.commandStr, "")

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
	ch    chan io.Reader
	once  sync.Once
	close chan struct{}
}

func newProxyReader() *proxyReader {
	return &proxyReader{
		ch:    make(chan io.Reader, 1),
		close: make(chan struct{}),
	}
}

func (p *proxyReader) Set(r io.Reader) {
	select {
	case p.ch <- r:
	default:
	}
}

func (p *proxyReader) Close() {
	p.once.Do(func() {
		close(p.close)
	})
}

func (p *proxyReader) Read(b []byte) (int, error) {
	if p.r == nil {
		select {
		case r := <-p.ch:
			p.r = r
		case <-p.close:
			return 0, io.EOF
		}
	}
	return p.r.Read(b)
}
