package ui

// mockCursorDetector is a test helper for simulating cursor position queries.
// Used by view_test.go and other tests that need a CursorDetector.
type mockCursorDetector struct {
	row int
	err error
}

func (m mockCursorDetector) GetCursorRow() (int, error) {
	return m.row, m.err
}
