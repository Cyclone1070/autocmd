package actionrouter

import (
	"context"
	"testing"
	"time"

	"github.com/Cyclone1070/autocmd/internal/domain"
)

func TestRouter_WaitDeliver(t *testing.T) {
	t.Run("deliver after wait", func(t *testing.T) {
		r := New()
		defer r.Close()
		callID := "test-call"
		act := domain.QuestionAnswerAction{CallID: callID}

		go func() {
			time.Sleep(10 * time.Millisecond)
			r.Deliver(act)
		}()

		got, err := r.Wait(context.Background(), callID)
		if err != nil {
			t.Fatalf("Wait failed: %v", err)
		}
		if q, ok := got.(domain.QuestionAnswerAction); !ok || q.CallID != callID {
			t.Errorf("got %#v, want %#v", got, act)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		r := New()
		defer r.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := r.Wait(ctx, "test-call")
		if err == nil {
			t.Fatal("expected error on cancelled context")
		}
	})

	t.Run("deliver before wait", func(t *testing.T) {
		r := New()
		defer r.Close()
		callID := "test-call-eager"
		act := domain.QuestionAnswerAction{CallID: callID}

		// Deliver FIRST
		r.Deliver(act)

		// Then Wait (it should scoop the pre-delivered action immediately)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		got, err := r.Wait(ctx, callID)
		if err != nil {
			t.Fatalf("Wait failed: %v", err)
		}
		if q, ok := got.(domain.QuestionAnswerAction); !ok || q.CallID != callID {
			t.Errorf("got %#v, want %#v", got, act)
		}
	})

	t.Run("discard stale actions", func(t *testing.T) {
		r := New()
		defer r.Close()

		staleAct := domain.QuestionAnswerAction{CallID: "stale-call"}
		correctAct := domain.QuestionAnswerAction{CallID: "fresh-call"}

		// Deliver stale action first (e.g. from an accidental previous multi-submit)
		r.Deliver(staleAct)

		// Deliver the valid action for the current executing tool
		r.Deliver(correctAct)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Waiting for the current tool should automatically discard 'stale-call'
		// and successfully retrieve 'fresh-call'.
		got, err := r.Wait(ctx, "fresh-call")
		if err != nil {
			t.Fatalf("Wait failed: %v", err)
		}

		if q, ok := got.(domain.QuestionAnswerAction); !ok || q.CallID != "fresh-call" {
			t.Errorf("expected fresh-call, got %#v", got)
		}

		// The FIFO should now be entirely empty.
		select {
		case <-r.out:
			t.Fatal("expected queue to be completely empty after processing")
		default:
		}
	})
}
