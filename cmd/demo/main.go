package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
)

func main() {
	cfg := config.DefaultConfig()
	renderer := ui.NewRenderer(os.Stdout, cfg)
	events := make(chan domain.Event, 10)

	go func() {
		for ev := range events {
			renderer.Send(ev)
		}
	}()

	go func() {
		defer close(events)
		time.Sleep(300 * time.Millisecond)

		// 1. Thinking
		events <- domain.ThinkingEvent{}
		time.Sleep(1200 * time.Millisecond)

		// 2. Fragmented Text (Streaming)
		textChunks := []string{
			"I'll help you ",
			"set up the ",
			"authentication system.\n\n",
			"Let me check the ",
			"**existing config",
			"** ",
			"first:\n\n",
			"```yaml\n",
			"auth:\n",
			"  enabled: ",
			"true\n",
			"  timeout: 30s",
			"\n```\n\n",
			"Okay, that looks ",
			"correct.",
		}
		for _, c := range textChunks {
			events <- domain.TextEvent{Text: c}
			time.Sleep(150 * time.Millisecond)
		}
		time.Sleep(600 * time.Millisecond)

		// 3. Tool: Read file
		events <- domain.ToolStartEvent{
			CallID:   "t1",
			ToolName: "read_file",
			Display:  domain.StringDisplay("Reading auth/config.yaml"),
		}
		time.Sleep(500 * time.Millisecond)
		events <- domain.ToolEndEvent{CallID: "t1"}
		time.Sleep(300 * time.Millisecond)

		// 4. More fragmented text
		moreChunks := []string{
			"The config looks ",
			"good. Now I'll ",
			"update the ",
			"*middleware*",
			" to add ",
			"proper token validation.",
		}
		for _, c := range moreChunks {
			events <- domain.TextEvent{Text: c}
			time.Sleep(100 * time.Millisecond)
		}
		time.Sleep(500 * time.Millisecond)

		// 5. PARALLEL TOOLS: Start two tools at the same time
		// Tool A: Reading a file
		events <- domain.ToolStartEvent{
			CallID:   "t2a",
			ToolName: "read_file",
			Display:  domain.StringDisplay("Reading middleware/auth.go"),
		}
		time.Sleep(100 * time.Millisecond)

		// Tool B: Edit file (starts while tool A is still running)
		events <- domain.ToolStartEvent{
			CallID:   "t2b",
			ToolName: "edit_file",
			Display: domain.DiffDisplay{
				Header:  "Edit middleware/auth.go",
				Added:   8,
				Removed: 2,
				Diff: `-func validateToken(token string) bool {
-    return token != ""
+func validateToken(token string) (*Claims, error) {
+    claims := &Claims{}
+    tkn, err := jwt.ParseWithClaims(token, claims, keyFunc)
+    if err != nil {
+        return nil, err
+    }
+    if !tkn.Valid {
+        return nil, ErrTokenExpired
+    }
+    return claims, nil
 }`,
			},
		}
		time.Sleep(600 * time.Millisecond)

		// Tool A completes first
		events <- domain.ToolEndEvent{CallID: "t2a"}
		time.Sleep(400 * time.Millisecond)

		// Tool B completes after
		events <- domain.ToolEndEvent{CallID: "t2b"}
		time.Sleep(300 * time.Millisecond)

		// 6. Text
		events <- domain.TextEvent{Text: `Now let me run the tests.`}
		time.Sleep(400 * time.Millisecond)

		// 7. Shell with realistic INCOMPLETE streaming chunks
		events <- domain.ToolStartEvent{
			CallID:   "t3",
			ToolName: "shell",
			Display: domain.ShellDisplay{
				Header:  "Running test suite",
				Command: "go test ./auth/... -v",
			},
		}
		time.Sleep(300 * time.Millisecond)

		// Simulating real streaming - incomplete lines, partial words, buffered output
		chunks := []string{
			"=== RUN   Test",
			"ValidateToken\n",
			"=== RUN   TestValidateToken/",
			"valid_token\n",
			"    auth_test.go:45: check",
			"ing token validati",
			"on...\n",
			"--- PASS: TestVal",
			"idateToken/valid_token (0.00s)\n",
			"=== RUN   TestValidateToken/expired",
			"_token\n",
			"    auth_test.go:52: simulating ",
			"expired token...\n--- PASS: TestValidateToken/expired_token (",
			"0.01s)\n",
			"=== RUN   TestValidate",
			"Token/invalid_signature\n",
			"--- PASS: TestValidateToken/invalid_signat",
			"ure (0.00s)\n",
			"--- PASS: TestValidateToken (0.02s",
			")\n",
			"=== RUN   TestRefreshToken\n",
			"    auth_test.go:78: generating refr",
			"esh token...\n",
			"--- PASS: TestRefreshToken (0.01s)\n",
			"PASS\n",
			"ok  \tgithub.com/example/app/auth\t",
			"0.034s\n",
		}
		for _, c := range chunks {
			events <- domain.ToolStreamEvent{CallID: "t3", Chunk: c}
			time.Sleep(80 * time.Millisecond)
		}
		events <- domain.ToolEndEvent{CallID: "t3"}
		time.Sleep(300 * time.Millisecond)

		// 8. Shell that fails with incomplete output
		events <- domain.ToolStartEvent{
			CallID:   "t4",
			ToolName: "shell",
			Display: domain.ShellDisplay{
				Header:  "Running integration tests",
				Command: "go test ./integration/... -v -tags=integration",
			},
		}
		time.Sleep(200 * time.Millisecond)

		failChunks := []string{
			"=== RUN   TestDatabase",
			"Connection\n",
			"    integration_test.go:23: connec",
			"ting to test database...\n",
			"    integration_test.go:28: dial tc",
			"p 127.0.0.1:5432: connect: ",
			"connection refused\n",
			"--- FAIL: TestDatabaseConnecti",
			"on (2.01s)\n",
			"FAIL\nexit status 1\n",
			"FAIL\tgithub.com/example/app/integr",
			"ation\t2.015s\n",
		}
		for _, c := range failChunks {
			events <- domain.ToolStreamEvent{CallID: "t4", Chunk: c}
			time.Sleep(100 * time.Millisecond)
		}
		events <- domain.ToolEndEvent{CallID: "t4", Error: "exit status 1"}
		time.Sleep(300 * time.Millisecond)

		// 9. Final summary
		events <- domain.TextEvent{Text: `The unit tests pass but integration tests failed (database not running - expected).

**Summary:**
- ✅ Updated token validation
- ✅ Unit tests passing  
- ⚠️ Integration tests require database`}
		time.Sleep(400 * time.Millisecond)

		events <- domain.DoneEvent{}
	}()

	renderer.Wait()
	fmt.Println("\nDemo complete.")
}
