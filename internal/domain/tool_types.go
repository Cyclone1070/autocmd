package domain

// Tool contract, invocations, JSON-serializable tool displays, and question-tool user actions.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cloudwego/eino/schema"
)

// ToolErrorCancelled is ToolDisplay.Error when Execute returns because the context was cancelled.
const ToolErrorCancelled = "Cancelled"

// ToolErrorFailed is the generic ToolDisplay.Error for any non-cancellation failures.
const ToolErrorFailed = "execution failed"

// Invocation is a validated, prepared tool call: at minimum a display for the UI.
// Returned by Tool.Prepare(). Concrete kinds implement ExecutableInvocation, StreamableInvocation,
// and/or InteractiveInvocation.
// Must be in domain so tools can implement without importing workflow.
type Invocation interface {
	Display() ToolDisplay
}

// ExecutableInvocation runs synchronously via Execute (most tools).
type ExecutableInvocation interface {
	Invocation
	// Execute runs the operation and returns LLM-facing content, the finalized display for UI/history,
	// and err. err is non-nil ONLY when the tool invocation itself is interrupted by context cancellation.
	// Technical failures (e.g. command errors, I/O failures) and semantic outcomes stay in content
	// and display without returning an err.
	Execute(ctx context.Context) (llmContent string, finalDisplay ToolDisplay, err error)
}

// StreamableInvocation is an ExecutableInvocation that exposes a live stdout/stderr stream for UI
// (e.g. bash.. The stream is not part of ToolDisplay so displays stay JSON-serializable.
type StreamableInvocation interface {
	ExecutableInvocation
	Stream() io.Reader
}

// InteractiveInvocation is resolved after the user replies via the UI (e.g. question tool).
// The executor must not call Execute on these; it waits for an action and calls Resolve instead.
type InteractiveInvocation interface {
	Invocation
	// Resolve is called after the user provides an action. It returns the finalized content and display.
	// Like Execute, it only returns a non-nil error if the context is cancelled.
	Resolve(ctx context.Context, action Action) (llmContent string, finalDisplay ToolDisplay, err error)
}

// ToolDisplay is implemented by all display types returned from tools.
// The UI uses type switches to render each type appropriately.
// Must be in domain so tools can implement without importing workflow.
type ToolDisplay interface {
	Type() string
	isToolDisplay()
	GetError() string
	WithError(err string) ToolDisplay
}

// StringDisplay is for simple text output (most tools).
type StringDisplay struct {
	TypeField string `json:"type"`
	Comment   string `json:"comment"`
	Content   string `json:"content"`
	Error     string `json:"error,omitempty"`
}

func (StringDisplay) isToolDisplay()     {}
func (s StringDisplay) Type() string     { return s.TypeField }
func (s StringDisplay) GetError() string { return s.Error }
func (s StringDisplay) WithError(err string) ToolDisplay {
	s.Error = err
	return s
}

// NewStringDisplay creates a new StringDisplay with correct type.
func NewStringDisplay(comment, content string) StringDisplay {
	return StringDisplay{TypeField: "string", Comment: comment, Content: content}
}

// DiffDisplay is for file edit operations with unified diff content.
type DiffDisplay struct {
	TypeField string `json:"type"`
	Comment   string `json:"comment"` // User-friendly description (e.g. "Updating auth")
	Target    string `json:"target"`  // Technical action (e.g. "Edit auth.go")
	Added     int    `json:"added"`   // Lines added
	Removed   int    `json:"removed"` // Lines removed
	Diff      string `json:"diff"`    // Unified diff content
	Error     string `json:"error,omitempty"`
}

func (DiffDisplay) isToolDisplay()     {}
func (d DiffDisplay) Type() string     { return d.TypeField }
func (d DiffDisplay) GetError() string { return d.Error }
func (d DiffDisplay) WithError(err string) ToolDisplay {
	d.Error = err
	return d
}

// NewDiffDisplay creates a new DiffDisplay with correct type.
func NewDiffDisplay(comment, target string, added, removed int, diff string) DiffDisplay {
	return DiffDisplay{
		TypeField: "diff",
		Comment:   comment,
		Target:    target,
		Added:     added,
		Removed:   removed,
		Diff:      diff,
	}
}

// BashDisplay is for bash command execution with streaming output.
type BashDisplay struct {
	TypeField      string `json:"type"`
	Comment        string `json:"comment"`         // Description from tool (e.g. "Installing dependencies")
	Command        string `json:"command"`         // The command being run (e.g. "npm install")
	CapturedOutput string `json:"captured_output"` // Raw output captured after execution (baked)
	Error          string `json:"error,omitempty"`
}

func (BashDisplay) isToolDisplay()     {}
func (s BashDisplay) Type() string     { return s.TypeField }
func (s BashDisplay) GetError() string { return s.Error }
func (s BashDisplay) WithError(err string) ToolDisplay {
	s.Error = err
	return s
}

// NewBashDisplay creates a new BashDisplay with correct type.
func NewBashDisplay(comment, command, capturedOutput string) BashDisplay {
	return BashDisplay{
		TypeField:      "bash",
		Comment:        comment,
		Command:        command,
		CapturedOutput: capturedOutput,
	}
}

// QuestionInfo describes one question in the question toolbox.
// Free-text answers are always offered by the UI (not part of this payload).
type QuestionInfo struct {
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	MultiSelect bool     `json:"multiSelect,omitempty"`
}

// QuestionDisplay is the tool UI payload for the question tool (preview and final baked state).
type QuestionDisplay struct {
	TypeField string         `json:"type"`
	Questions []QuestionInfo `json:"questions"`
	Error     string         `json:"error,omitempty"`
}

func (QuestionDisplay) isToolDisplay()     {}
func (d QuestionDisplay) Type() string     { return d.TypeField }
func (d QuestionDisplay) GetError() string { return d.Error }
func (d QuestionDisplay) WithError(err string) ToolDisplay {
	d.Error = err
	return d
}

// NewQuestionDisplay returns a QuestionDisplay with type "question".
func NewQuestionDisplay(questions []QuestionInfo) QuestionDisplay {
	return QuestionDisplay{TypeField: "question", Questions: questions}
}

// QuestionAnswerAction is sent by the prompt UI after the user submits or cancels the question toolbox.
type QuestionAnswerAction struct {
	CallID  string
	Answers [][]string // per-question selected labels and/or custom text; order matches Questions
}

func (QuestionAnswerAction) isAction() {}

// GetCallID implements CallIDer for action routing.
func (a QuestionAnswerAction) GetCallID() string { return a.CallID }

// ToolDisplays is a helper type for polymorphic JSON unmarshaling of ToolDisplay maps.
type ToolDisplays map[string]ToolDisplay

func (m *ToolDisplays) UnmarshalJSON(data []byte) error {
	var raws map[string]json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}

	*m = make(ToolDisplays)
	for id, raw := range raws {
		var peek struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &peek); err != nil {
			return err
		}

		var display ToolDisplay
		switch peek.Type {
		case "string":
			var d StringDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		case "diff":
			var d DiffDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		case "bash":
			var d BashDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		case "question":
			var d QuestionDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		default:
			return fmt.Errorf("unknown display type: %s", peek.Type)
		}
		(*m)[id] = display
	}
	return nil
}

// Tool defines the interface for individual tools.
type Tool interface {
	Name() string
	IsConcurrentSafe() bool
	Definition() *schema.ToolInfo
	Prepare(params string) (Invocation, error)
}
