// Package cursor provides terminal cursor position detection.
// Used by compose to resolve geometry for engine layout.

package cursor

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

// CursorDetector abstracts the ability to query the current cursor position.
type CursorDetector interface {
	GetCursorRow() (int, error)
}

const cursorResponseTimeout = 100 * time.Millisecond

// TerminalCursorDetector implements CursorDetector using VT100 escape codes.
type TerminalCursorDetector struct {
	In  io.Reader
	Out io.Writer
}

// NewTerminalCursorDetector creates a new detector using the given I/O.
func NewTerminalCursorDetector(in io.Reader, out io.Writer) *TerminalCursorDetector {
	return &TerminalCursorDetector{
		In:  in,
		Out: out,
	}
}

// GetCursorRow returns the current 1-based row of the cursor.
func (d *TerminalCursorDetector) GetCursorRow() (int, error) {
	file, ok := d.In.(*os.File)
	if ok {
		oldState, err := term.MakeRaw(int(file.Fd()))
		if err != nil {
			return 0, fmt.Errorf("failed to enable raw mode: %w", err)
		}
		defer term.Restore(int(file.Fd()), oldState)
	}

	if _, err := d.Out.Write([]byte("\033[6n")); err != nil {
		return 0, fmt.Errorf("failed to write cursor request: %w", err)
	}

	ch := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		reader := bufio.NewReader(d.In)
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
	case <-time.After(cursorResponseTimeout):
		return 0, fmt.Errorf("timeout waiting for cursor response")
	}
}

func parseCursorResponse(response string) (int, error) {
	if !strings.HasSuffix(response, "R") {
		return 0, fmt.Errorf("invalid cursor response: missing R terminator: %s", response)
	}

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
