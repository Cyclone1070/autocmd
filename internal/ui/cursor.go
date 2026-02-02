package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// GetCursorRow returns the current 1-based row of the cursor.
// It uses VT100 escape codes to query the terminal.
// Helper uses direct terminal I/O (not Bubble Tea) because it runs before the model starts.
func GetCursorRow() (int, error) {
	// Enable raw mode to read response without Enter
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return 0, fmt.Errorf("failed to enable raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Send cursor position request
	// \033[6n requests "Device Status Report" specifically for cursor position
	if _, err := os.Stdout.Write([]byte("\033[6n")); err != nil {
		return 0, fmt.Errorf("failed to write cursor request: %w", err)
	}

	// Read response with timeout
	// Response format: \033[<row>;<col>R
	ch := make(chan string)
	errCh := make(chan error)

	go func() {
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('R')
		if err != nil {
			errCh <- err
			return
		}
		ch <- response
	}()

	select {
	case response := <-ch:
		return parseCursorResponse(response)
	case err := <-errCh:
		return 0, fmt.Errorf("failed to read cursor response: %w", err)
	case <-time.After(100 * time.Millisecond):
		return 0, fmt.Errorf("timeout waiting for cursor response")
	}
}

func parseCursorResponse(response string) (int, error) {
	// Expected format: \x1b[24;1R (where 24 is row, 1 is col)
	// Strip "R" and "\x1b["
	content := strings.TrimSuffix(response, "R")
	idx := strings.LastIndex(content, "[")
	if idx == -1 {
		return 0, fmt.Errorf("invalid cursor response format: %s", response)
	}

	coords := content[idx+1:]
	parts := strings.Split(coords, ";")
	if len(parts) != 2 {
		return 0, fmt.Errorf("unexpected cursor response parts: %s", coords)
	}

	row, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("failed to parse row: %w", err)
	}

	return row, nil
}
