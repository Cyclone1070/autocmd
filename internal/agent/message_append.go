package agent

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// appendMessageMerge appends msg to *msgs, merging with the last element when
// both are user or both are assistant: trimmed Content joined with a blank line,
// plus schema.ConcatMessages for other fields. Any other role (tool, system, etc.)
// is appended without merging.
func appendMessageMerge(msgs *[]*schema.Message, msg *schema.Message) error {
	if msgs == nil {
		return fmt.Errorf("messages slice pointer is nil")
	}
	if msg == nil {
		return fmt.Errorf("message is nil")
	}
	if len(*msgs) == 0 {
		*msgs = append(*msgs, msg)
		return nil
	}
	if msg.Role != schema.User && msg.Role != schema.Assistant {
		*msgs = append(*msgs, msg)
		return nil
	}
	last := (*msgs)[len(*msgs)-1]
	if last == nil {
		*msgs = append(*msgs, msg)
		return nil
	}
	if last.Role == msg.Role {
		merged, err := mergeSameRoleMessages(last, msg)
		if err != nil {
			return err
		}
		(*msgs)[len(*msgs)-1] = merged
		return nil
	}
	*msgs = append(*msgs, msg)
	return nil
}

// mergeSameRoleMessages joins two user or two assistant messages: trim each Content, then
// join with a blank line. Other fields are merged via schema.ConcatMessages.
func mergeSameRoleMessages(last, msg *schema.Message) (*schema.Message, error) {
	aTrim := strings.TrimSpace(last.Content)
	bTrim := strings.TrimSpace(msg.Content)

	leftMsg := *last
	rightMsg := *msg
	switch {
	case aTrim != "" && bTrim != "":
		leftMsg.Content = aTrim + "\n\n"
		rightMsg.Content = bTrim
	case aTrim == "":
		leftMsg.Content = ""
		rightMsg.Content = bTrim
	default:
		leftMsg.Content = aTrim
		rightMsg.Content = ""
	}

	merged, err := schema.ConcatMessages([]*schema.Message{&leftMsg, &rightMsg})
	if err != nil {
		return nil, err
	}
	return merged, nil
}
