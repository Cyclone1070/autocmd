package cursor

import (
	"testing"
)

func TestParseCursorResponse_StandardResponseRow1(t *testing.T) {
	row, err := parseCursorResponse("\x1b[1;1R")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row != 1 {
		t.Errorf("got row %d, want 1", row)
	}
}

func TestParseCursorResponse_StandardResponseRow24(t *testing.T) {
	row, err := parseCursorResponse("\x1b[24;1R")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row != 24 {
		t.Errorf("got row %d, want 24", row)
	}
}

func TestParseCursorResponse_LargeRowNumber(t *testing.T) {
	row, err := parseCursorResponse("\x1b[999;50R")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row != 999 {
		t.Errorf("got row %d, want 999", row)
	}
}

func TestParseCursorResponse_DifferentColumnValues(t *testing.T) {
	row, err := parseCursorResponse("\x1b[10;80R")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row != 10 {
		t.Errorf("got row %d, want 10", row)
	}
}

func TestParseCursorResponse_WithExtraPrefix(t *testing.T) {
	row, err := parseCursorResponse("garbage\x1b[5;1R")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row != 5 {
		t.Errorf("got row %d, want 5", row)
	}
}

func TestParseCursorResponse_MissingRTerminator(t *testing.T) {
	_, err := parseCursorResponse("\x1b[24;1")
	if err == nil {
		t.Error("expected error for missing R terminator")
	}
}

func TestParseCursorResponse_MissingBracket(t *testing.T) {
	_, err := parseCursorResponse("24;1R")
	if err == nil {
		t.Error("expected error for missing bracket")
	}
}

func TestParseCursorResponse_MissingSemicolon(t *testing.T) {
	_, err := parseCursorResponse("\x1b[241R")
	if err == nil {
		t.Error("expected error for missing semicolon")
	}
}

func TestParseCursorResponse_EmptyString(t *testing.T) {
	_, err := parseCursorResponse("")
	if err == nil {
		t.Error("expected error for empty string")
	}
}

func TestParseCursorResponse_OnlyR(t *testing.T) {
	_, err := parseCursorResponse("R")
	if err == nil {
		t.Error("expected error for only R")
	}
}

func TestParseCursorResponse_NonNumericRow(t *testing.T) {
	_, err := parseCursorResponse("\x1b[abc;1R")
	if err == nil {
		t.Error("expected error for non-numeric row")
	}
}

func TestParseCursorResponse_TooManyParts(t *testing.T) {
	_, err := parseCursorResponse("\x1b[1;2;3R")
	if err == nil {
		t.Error("expected error for too many parts")
	}
}

func TestParseCursorResponse_OnlyBracketAndR(t *testing.T) {
	_, err := parseCursorResponse("\x1b[R")
	if err == nil {
		t.Error("expected error for only bracket and R")
	}
}
