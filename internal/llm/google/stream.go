package google

import (
	"encoding/json"
	"iter"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/google/uuid"
	"google.golang.org/genai"
)

type googleStream struct {
	iter    iter.Seq2[*genai.GenerateContentResponse, error]
	pull    func() (*genai.GenerateContentResponse, error, bool)
	stop    func()
	current domain.StreamChunk
	buffer  []domain.StreamChunk
	err     error
}

func (s *googleStream) Next() bool {
	if len(s.buffer) > 0 {
		s.current = s.buffer[0]
		s.buffer = s.buffer[1:]
		return true
	}

	if s.pull == nil {
		s.pull, s.stop = iter.Pull2(s.iter)
	}

	for {
		resp, err, ok := s.pull()
		if !ok {
			s.stop()
			return false
		}

		if err != nil {
			s.err = err
			s.stop()
			return false
		}

		chunks := toChunks(resp)
		if len(chunks) > 0 {
			s.current = chunks[0]
			s.buffer = chunks[1:]
			return true
		}
		// If empty chunks (e.g. just safety ratings?), loop and get next
	}
}

func (s *googleStream) Chunk() domain.StreamChunk {
	return s.current
}

func (s *googleStream) Err() error {
	return s.err
}

func toChunks(resp *genai.GenerateContentResponse) []domain.StreamChunk {
	if resp == nil {
		return nil
	}

	var chunks []domain.StreamChunk

	if len(resp.Candidates) == 0 {
		return chunks
	}

	cand := resp.Candidates[0]
	if cand.Content == nil {
		return chunks
	}

	for _, part := range cand.Content.Parts {
		thoughtSig := string(part.ThoughtSignature)

		if part.Text != "" {
			chunks = append(chunks, domain.TextChunk{
				Text:             part.Text,
				IsThought:        part.Thought,
				ThoughtSignature: thoughtSig,
			})
		}
		if part.FunctionCall != nil {
			args, err := json.Marshal(part.FunctionCall.Args)
			if err != nil {
				// This should technically never happen with map[string]any from SDK
				// but we handle it to avoid silent data loss if we log it or similar.
				// For now let's just use empty JSON if it fails.
				args = json.RawMessage("{}")
			}
			chunks = append(chunks, domain.ToolCall{
				ID:               uuid.NewString(),
				Name:             part.FunctionCall.Name,
				Arguments:        args,
				ThoughtSignature: thoughtSig,
			})
		}
	}
	return chunks
}
