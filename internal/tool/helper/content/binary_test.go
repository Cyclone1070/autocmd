package content

import "testing"

func TestIsBinaryContent(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		expected bool
	}{
		{
			name:     "EmptyInput",
			content:  []byte{},
			expected: false,
		},
		{
			name:     "ASCIIText",
			content:  []byte("hello world"),
			expected: false,
		},
		{
			name:     "UTF8Multibyte",
			content:  []byte("こんにちは"),
			expected: false,
		},
		{
			name:     "NullByteAtStart",
			content:  []byte{0, 'a', 'b'},
			expected: true,
		},
		{
			name:     "NullByteAtEnd7999",
			content:  makeNullByteAt(7999), // 0-indexed, so 8000th byte
			expected: true,
		},
		{
			name:     "NullByteAt8000Ignored",
			content:  makeNullByteAt(8000), // 8001st byte (beyond sample size)
			expected: false,
		},
		{
			name:     "UTF16LEBOMNotBinary",
			content:  []byte{0xFF, 0xFE, 'a', 0x00}, // Valid UTF-16 LE text
			expected: false,
		},
		{
			name:     "UTF16BEBOMNotBinary",
			content:  []byte{0xFE, 0xFF, 0x00, 'a'}, // Valid UTF-16 BE text
			expected: false,
		},
		{
			name:     "UTF32LEBOMNotBinary",
			content:  []byte{0xFF, 0xFE, 0x00, 0x00, 'a', 0x00, 0x00, 0x00}, // Valid UTF-32 LE text
			expected: false,
		},
		{
			name:     "UTF32BEBOMNotBinary",
			content:  []byte{0x00, 0x00, 0xFE, 0xFF, 0x00, 0x00, 0x00, 'a'}, // Valid UTF-32 BE text
			expected: false,
		},
		{
			name:     "SingleNullByte",
			content:  []byte{0},
			expected: true,
		},
		{
			name:     "SingleCharByte",
			content:  []byte{'a'},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBinaryContent(tt.content); got != tt.expected {
				t.Errorf("IsBinaryContent() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Helper to create a byte slice with a null byte at a specific index
// All other bytes are 'a'.
func makeNullByteAt(index int) []byte {
	// Make slice large enough to include the index
	b := make([]byte, index+1)
	for i := range b {
		b[i] = 'a'
	}
	b[index] = 0
	return b
}
