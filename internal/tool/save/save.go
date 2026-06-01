// Package save provides a tool for the AI agent to save bash commands for later reuse.
package save

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/runtimectx"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const toolName = "save_command"

type commandSaver interface {
	Get(name string) (*domain.SavedCommand, bool)
	Save(name, command, description string) error
}

// Tool saves a command name-to-command mapping for the user to run later via CLI.
type Tool struct {
	saver commandSaver
}

// NewTool creates a save_command tool.
func NewTool(saver commandSaver) *Tool {
	if saver == nil {
		panic("saver is required")
	}
	return &Tool{saver: saver}
}

func (t *Tool) IsConcurrentSafe() bool { return true }

func (t *Tool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: toolName,
		Desc: `Saves a bash command under a user-facing name so the user can run it later by typing "autocmd <name>" without going through the AI loop.

IMPORTANT: Never overwrite an existing command unless the user explicitly asks you to. The "override" parameter defaults to false for a reason. Always inform the user when a name is taken and ask for their permission before setting override=true.

Before saving a read-only command (inspection, display, etc.), test it first to verify the output looks correct and the command runs without errors.
For commands with side effects (writes, deletes, modifications), do NOT save them automatically — ask the user for explicit permission first. Make sure the user understands what the command does before saving.

Use this tool when the user asks you to save a command they might want to run again later. Examples:
- "Save this command so I can use it later" → call save_command with a short name and the full bash command.
- After figuring out the right incantation for something complex (compiling, git operations, docker commands, etc.), offer to save it.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"name": {
				Type:     schema.String,
				Desc:     "The short name the user will type to run this command (e.g. 'git imp', 'compile my app'). Can contain spaces.",
				Required: true,
			},
			"command": {
				Type:     schema.String,
				Desc:     "The actual bash command to execute when the user runs this saved command (e.g. 'git status --porcelain').",
				Required: true,
			},
			"description": {
				Type:     schema.String,
				Desc:     "Optional human-readable description of what this command does.",
				Required: false,
			},
			"override": {
				Type:     schema.Boolean,
				Desc:     "Set to true to overwrite an existing command with the same name. Defaults to false. When false, saving a name that already exists will return an error.",
				Required: false,
			},
		}),
	}, nil
}

type toolParams struct {
	Name        string `json:"name"`
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
	Override    bool   `json:"override"`
}

func (t *Tool) validate(argumentsInJSON string) (*toolParams, error) {
	var p toolParams
	if err := json.Unmarshal([]byte(argumentsInJSON), &p); err != nil {
		return nil, fmt.Errorf("failed to parse save params: %w", err)
	}
	if strings.TrimSpace(p.Name) == "" {
		return nil, fmt.Errorf("name must not be empty")
	}
	if strings.TrimSpace(p.Command) == "" {
		return nil, fmt.Errorf("command must not be empty")
	}
	return &p, nil
}

func buildDisplay(p *toolParams) domain.StringDisplay {
	return domain.NewStringDisplay(
		fmt.Sprintf("Save %q command", p.Name),
		p.Command,
	)
}

func (t *Tool) execute(ctx context.Context, p *toolParams) (string, domain.ToolDisplay) {
	d := buildDisplay(p)

	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, d
	}

	_, exists := t.saver.Get(p.Name)
	if exists && !p.Override {
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("command %q already exists. Ask the user for explicit permission before overwriting it. If they agree, call save_command again with override=true.", p.Name), d
	}

	if err := t.saver.Save(p.Name, p.Command, p.Description); err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		}
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: failed to save command: %v", err), d
	}

	var msg string
	if exists && p.Override {
		msg = fmt.Sprintf("Updated command %q → `%s`", p.Name, p.Command)
	} else {
		msg = fmt.Sprintf("Saved command %q → `%s`", p.Name, p.Command)
	}
	if p.Description != "" {
		msg += fmt.Sprintf(" (%s)", p.Description)
	}
	return msg, d
}

func (t *Tool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	p, err := t.validate(argumentsInJSON)
	if err != nil {
		return "", err
	}
	callID := compose.GetToolCallID(ctx)
	llmContent, finalDisplay := t.execute(ctx, p)
	if events, ok := runtimectx.EventSenderFrom(ctx); ok && events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: finalDisplay})
	}
	if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok && sink != nil {
		sink(callID, finalDisplay)
	}
	return llmContent, nil
}

func (t *Tool) Preview(input *compose.ToolInput) domain.ToolDisplay {
	var p toolParams
	if err := json.Unmarshal([]byte(input.Arguments), &p); err != nil || p.Name == "" {
		return domain.NewStringDisplay(fmt.Sprintf("Run %q", toolName), "")
	}
	return buildDisplay(&p)
}

func (t *Tool) PreflightValidate(input *compose.ToolInput) error {
	_, err := t.validate(input.Arguments)
	return err
}
