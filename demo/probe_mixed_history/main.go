package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"os"
	"time"

	"github.com/Cyclone1070/iav/cmd"
	"github.com/cloudwego/eino/schema"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "probe failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	deps, err := cmd.Wire()
	if err != nil {
		return fmt.Errorf("wire deps: %w", err)
	}
	modelID := deps.State.Model()
	if modelID == "" {
		return fmt.Errorf("no model selected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	llm, err := deps.LLMRegistry.Get(ctx, modelID)
	if err != nil {
		return fmt.Errorf("get llm %q: %w", modelID, err)
	}

	msgs, err := buildProbeMessages()
	if err != nil {
		return err
	}

	withTools, err := llm.Model().WithTools([]*schema.ToolInfo{
		{
			Name: "bash",
			Desc: "Run shell command",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"command": {
					Type:     schema.String,
					Required: true,
				},
				"description": {
					Type: schema.String,
				},
			}),
		},
	})
	if err != nil {
		return fmt.Errorf("bind tools failed: %w", err)
	}

	streamStart := time.Now()
	stream, err := withTools.Stream(ctx, msgs)
	if err != nil {
		return fmt.Errorf("stream call failed: %w", err)
	}
	defer stream.Close()
	fmt.Printf("[t=%6.3fs] stream returned\n", time.Since(streamStart).Seconds())

	first := true
	chunks := 0
	final := &schema.Message{}
	for {
		msg, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("stream recv (chunk %d): %w", chunks, err)
		}
		if first {
			fmt.Printf("[t=%6.3fs] first chunk role=%s contentLen=%d reasoningLen=%d toolCalls=%d\n",
				time.Since(streamStart).Seconds(), msg.Role, len(msg.Content), len(msg.ReasoningContent), len(msg.ToolCalls))
			first = false
		}
		final.Role = msg.Role
		final.Content += msg.Content
		final.ReasoningContent += msg.ReasoningContent
		final.ToolCalls = append(final.ToolCalls, msg.ToolCalls...)
		chunks++
	}
	fmt.Printf("[t=%6.3fs] stream done, %d chunks\n", time.Since(streamStart).Seconds(), chunks)

	out, _ := json.MarshalIndent(struct {
		ModelID          string `json:"model_id"`
		Role             string `json:"role"`
		ContentLen       int    `json:"content_len"`
		ReasoningLen     int    `json:"reasoning_len"`
		ToolCallsLen     int    `json:"tool_calls_len"`
		FinishReasonHint any    `json:"response_meta"`
	}{
		ModelID:          modelID,
		Role:             string(final.Role),
		ContentLen:       len(final.Content),
		ReasoningLen:     len(final.ReasoningContent),
		ToolCallsLen:     len(final.ToolCalls),
		FinishReasonHint: final.ResponseMeta,
	}, "", "  ")
	fmt.Println(string(out))
	return nil
}

func buildProbeMessages() ([]*schema.Message, error) {
	if replayPath := os.Getenv("PROBE_REPLAY_MESSAGES"); replayPath != "" {
		type replay struct {
			Messages []*schema.Message `json:"messages"`
		}
		raw, err := os.ReadFile(replayPath)
		if err != nil {
			return nil, fmt.Errorf("read replay file: %w", err)
		}
		var r replay
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("decode replay file: %w", err)
		}
		if len(r.Messages) < 3 {
			return nil, fmt.Errorf("replay file has %d messages; need at least 3", len(r.Messages))
		}
		msgs := r.Messages[:3]
		if os.Getenv("PROBE_STRIP_REASONING") == "1" && len(msgs) > 1 {
			msgs[1].ReasoningContent = ""
		}
		if os.Getenv("PROBE_STRIP_ASSISTANT_CONTENT") == "1" && len(msgs) > 1 {
			msgs[1].Content = ""
		}
		if os.Getenv("PROBE_SET_TOOLCALL_TYPE") == "1" && len(msgs) > 1 {
			for i := range msgs[1].ToolCalls {
				msgs[1].ToolCalls[i].Type = "function"
			}
		}
		if os.Getenv("PROBE_STRIP_TOOLCALL_EXTRA") == "1" && len(msgs) > 1 {
			for i := range msgs[1].ToolCalls {
				msgs[1].ToolCalls[i].Extra = nil
			}
		}
		return msgs, nil
	}

	idx := 0
	toolPayload := "AGENTS.md\ncmd\ndemo\ninternal\n"
	if raw := os.Getenv("PROBE_TOOL_BYTES"); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil {
			return nil, fmt.Errorf("invalid PROBE_TOOL_BYTES: %w", convErr)
		}
		if n > 0 {
			toolPayload = strings.Repeat("x", n)
		}
	}

	return []*schema.Message{
		{
			Role:    schema.User,
			Content: "Please call bash and then continue.",
		},
		{
			Role:    schema.Assistant,
			Content: "Sure, I will run bash now.",
			ReasoningContent: "Need to call bash tool first, then continue.",
			ToolCalls: []schema.ToolCall{
				{
					Index: &idx,
					ID:    "probe-call-1",
					Function: schema.FunctionCall{
						Name:      "bash",
						Arguments: `{"command":"ls","description":"list files"}`,
					},
				},
			},
		},
		{
			Role:       schema.Tool,
			ToolCallID: "probe-call-1",
			ToolName:   "bash",
			Content:    toolPayload,
		},
	}, nil
}
