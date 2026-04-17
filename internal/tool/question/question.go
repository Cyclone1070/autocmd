package question

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

type QuestionTool struct{}

func NewQuestionTool() *QuestionTool {
	return &QuestionTool{}
}

func (t *QuestionTool) Name() string {
	return "ask_question"
}

func (t *QuestionTool) IsConcurrentSafe() bool { return false }

func (t *QuestionTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: `Asks the user one or more interactive questions to gather information, clarify ambiguity, understand preferences, make decisions or offer them choices.

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
							Desc:     "The prompt/question string visible to the user.",
							Required: true,
						},
						"options": {
							Type: schema.Array,
							ElemInfo: &schema.ParameterInfo{
								Type: schema.String,
							},
							Desc: "Optional list of fixed choices for selection.",
						},
						"multiSelect": {
							Type: schema.Boolean,
							Desc: "Whether the user can select multiple options.",
						},
					},
					Required: true,
				},
				Desc:     "The list of interactive questions to ask.",
				Required: true,
			},
		}),
	}
}

type toolParams struct {
	Questions []domain.QuestionInfo `json:"questions"`
}

func (t *QuestionTool) Prepare(params string) (domain.Invocation, error) {
	var p toolParams
	if err := json.Unmarshal([]byte(params), &p); err != nil {
		return nil, fmt.Errorf("failed to parse question params: %w", err)
	}

	return &QuestionInvocation{questions: p.Questions}, nil
}

type QuestionInvocation struct {
	questions []domain.QuestionInfo
}

func (i *QuestionInvocation) Display() domain.ToolDisplay {
	return domain.NewQuestionDisplay(i.questions)
}

func (i *QuestionInvocation) Resolve(ctx context.Context, action domain.Action) (string, domain.ToolDisplay) {
	if ctx.Err() != nil {
		list := i.formatQuestionList()
		display := domain.NewStringDisplay("Questions attempted", list)
		display.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, display
	}

	qa, ok := action.(domain.QuestionAnswerAction)
	if !ok {
		panic(fmt.Sprintf("QuestionInvocation.Resolve: expected QuestionAnswerAction, got %T", action))
	}

	summary := formatAnswerSummary(i.questions, qa.Answers)

	return summary, domain.NewStringDisplay("Questions attempted", summary)
}

func (i *QuestionInvocation) formatQuestionList() string {
	var out []string
	for _, q := range i.questions {
		out = append(out, "Q: "+q.Question)
	}
	return strings.Join(out, "\n\n")
}

func formatAnswerSummary(questions []domain.QuestionInfo, answers [][]string) string {
	var sb strings.Builder
	for idx, q := range questions {
		ans := "Not answered"
		if idx < len(answers) && len(answers[idx]) > 0 {
			ans = strings.Join(answers[idx], ", ")
		}
		fmt.Fprintf(&sb, "Q: %s\nA: %s\n\n", q.Question, ans)
	}
	return strings.TrimSpace(sb.String())
}
