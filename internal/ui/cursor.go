package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// cursorResponseTimeout is the deadline for receiving VT100 Device Status Report.
// 100ms is sufficient for local terminals; only fails on broken/non-responsive terminals.
const cursorResponseTimeout = 100 * time.Millisecond

// TerminalCursorDetector implements CursorDetector using VT100 escape codes.
type TerminalCursorDetector struct {
	In  io.Reader
	Out io.Writer
}

// NewTerminalCursorDetector creates a new detector using the given I/O.
// Typically In is os.Stdin and Out is os.Stdout.
func NewTerminalCursorDetector(in io.Reader, out io.Writer) *TerminalCursorDetector {
	return &TerminalCursorDetector{
		In:  in,
		Out: out,
	}
}

// GetCursorRow returns the current 1-based row of the cursor.
// It uses VT100 escape codes to query the terminal.
func (d *TerminalCursorDetector) GetCursorRow() (int, error) {
	// Enable raw mode to read response without Enter
	file, ok := d.In.(*os.File)
	if ok {
		// Use raw mode if possible
		oldState, err := term.MakeRaw(int(file.Fd()))
		if err != nil {
			return 0, fmt.Errorf("failed to enable raw mode: %w", err)
		}
		defer term.Restore(int(file.Fd()), oldState)

		// Set read deadline to avoid hanging forever
		if err := file.SetReadDeadline(time.Now().Add(cursorResponseTimeout)); err != nil {
			return 0, fmt.Errorf("failed to set read deadline: %w", err)
		}
		// Ensure we clear the deadline (though we are done reading anyway/file is deferred)
		// It's good practice.
		defer file.SetReadDeadline(time.Time{})
	}

	// Send cursor position request
	// \033[6n requests "Device Status Report" specifically for cursor position
	if _, err := d.Out.Write([]byte("\033[6n")); err != nil {
		return 0, fmt.Errorf("failed to write cursor request: %w", err)
	}

	// Read response directly (blocking but with timeout if file)
	reader := bufio.NewReader(d.In)
	response, err := reader.ReadString('R')
	if err != nil {
		if os.IsTimeout(err) {
			return 0, fmt.Errorf("timeout waiting for cursor response")
		}
		return 0, fmt.Errorf("failed to read cursor response: %w", err)
	}

	return parseCursorResponse(response)
}

func parseCursorResponse(response string) (int, error) {
	// Expected format: \x1b[24;1R (where 24 is row, 1 is col)

	// Validate response ends with 'R'
	if !strings.HasSuffix(response, "R") {
		return 0, fmt.Errorf("invalid cursor response: missing R terminator: %s", response)
	}

	// Strip "R" and find the bracket
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
