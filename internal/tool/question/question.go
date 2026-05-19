// Package question provides a tool for asking the user interactive questions.
package question

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const toolName = "ask_question"

// Tool is a tool that allows the LLM to ask the user interactive questions.
type Tool struct{}

// NewTool creates a new Tool.
func NewTool() *Tool {
	return &Tool{}
}

// IsConcurrentSafe indicates if the tool can be run concurrently (false for interactive tools).
func (t *Tool) IsConcurrentSafe() bool { return false }

func (t *Tool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: toolName,
		Desc: `Asks the user one or more multiple choice questions to gather information, clarify ambiguity, understand preferences, make decisions or offer them choices.

Use this tool when you need to ask the user questions during execution. This allows you to:
1. Gather user preferences or requirements.
2. Clarify ambiguous instructions.
3. Get decisions on implementation choices as you work.
4. Offer choices to the user about what direction to take.

Usage notes:
- Users will always be able to provide custom text input even if options are provided.
- Use multiSelect: true to allow multiple answers to be selected for a question.
- If you recommend a specific option, make that the first option in the list and add "(Recommended)" at the end of the label.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"questions": {
				Type: schema.Array,
				ElemInfo: &schema.ParameterInfo{
					Type: schema.Object,
					SubParams: map[string]*schema.ParameterInfo{
						"question": {
							Type:     schema.String,
							Desc:     "The complete question to ask the user. Should be clear, specific, and end with a question mark. Example: \"Which library should we use for date formatting?\" If multiSelect is true, phrase it accordingly, e.g. \"Which features do you want to enable?\"",
							Required: true,
						},
						"options": {
							Type: schema.Array,
							ElemInfo: &schema.ParameterInfo{
								Type: schema.String,
							},
							Desc: "The available choices for this question. Must have 2-4 options. Each option should be a distinct, mutually exclusive choice (unless multiSelect is enabled). There should be no 'Other' option, that will be provided automatically.",
						},
						"multiSelect": {
							Type: schema.Boolean,
							Desc: "Set to true to allow the user to select multiple options instead of just one. Use when choices are not mutually exclusive.",
						},
					},
					Required: true,
				},
				Desc:     "The list of multiple choice questions to ask (1-4 questions).",
				Required: true,
			},
		}),
	}, nil
}

func (t *Tool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	questions, err := t.validate(argumentsInJSON)
	if err != nil {
		return "", err
	}
	waiter, ok := runtimectx.ActionWaiterFrom(ctx)
	if !ok || waiter == nil {
		callID := compose.GetToolCallID(ctx)
		failed := domain.NewStringDisplay("Question attempted", "").WithError(domain.ToolErrorFailed)
		if events, ok := runtimectx.EventSenderFrom(ctx); ok && events != nil {
			events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: failed})
		}
		if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok && sink != nil {
			sink(callID, failed)
		}
		return "", fmt.Errorf("question tool requires action waiter")
	}
	action, err := waiter.Wait(ctx, compose.GetToolCallID(ctx))
	if err != nil {
		callID := compose.GetToolCallID(ctx)
		toolErr := domain.ToolErrorFailed
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			toolErr = domain.ToolErrorCancelled
		}
		failed := domain.NewStringDisplay("Question attempted", "").WithError(toolErr)
		if events, ok := runtimectx.EventSenderFrom(ctx); ok && events != nil {
			events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: failed})
		}
		if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok && sink != nil {
			sink(callID, failed)
		}
		return "", err
	}
	callID := compose.GetToolCallID(ctx)
	llmContent, finalDisplay := resolveQuestions(ctx, questions, action)
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
	if err := json.Unmarshal([]byte(input.Arguments), &p); err != nil {
		return domain.NewQuestionDisplay(nil)
	}
	return domain.NewQuestionDisplay(p.Questions)
}

func (t *Tool) PreflightValidate(input *compose.ToolInput) error {
	_, err := t.validate(input.Arguments)
	return err
}

type toolParams struct {
	Questions []domain.QuestionInfo `json:"questions"`
}

func (t *Tool) validate(params string) ([]domain.QuestionInfo, error) {
	var p toolParams
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return nil, fmt.Errorf("failed to parse question params: %w", err)
	}
	return p.Questions, nil
}

func resolveQuestions(ctx context.Context, questions []domain.QuestionInfo, action domain.Action) (string, domain.ToolDisplay) {
	if ctx.Err() != nil {
		display := domain.NewStringDisplay("Question attempted", "")
		display.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, display
	}

	qa, ok := action.(domain.QuestionAnswerAction)
	if !ok {
		panic(fmt.Sprintf("invocation.Resolve: expected QuestionAnswerAction, got %T", action))
	}

	summary := formatAnswerSummary(questions, qa.Answers)
	skipped := countSkippedQuestions(questions, qa.Answers)
	if summary == "" {
		return "User has skipped all questions.", domain.NewStringDisplay("Questions skipped", "")
	}
	llmContent := fmt.Sprintf("User has answered your questions (%d skipped):\n\n%s\n\nYou can now continue with the user's answers in mind.", skipped, summary)
	return llmContent, domain.NewStringDisplay("Questions answered", summary)
}

func formatAnswerSummary(questions []domain.QuestionInfo, answers [][]string) string {
	var sb strings.Builder
	for idx, q := range questions {
		if idx >= len(answers) || len(answers[idx]) == 0 {
			continue
		}
		ans := strings.Join(answers[idx], ", ")
		fmt.Fprintf(&sb, "Q: %s\nA: %s\n\n", q.Question, ans)
	}
	return strings.TrimSpace(sb.String())
}

func countSkippedQuestions(questions []domain.QuestionInfo, answers [][]string) int {
	skipped := 0
	for idx := range questions {
		if idx >= len(answers) || len(answers[idx]) == 0 {
			skipped++
		}
	}
	return skipped
}
