package content

// binarySampleSize defines the number of bytes to scan for null bytes when detecting binary content.
// This matches Git's heuristic (8000 bytes since 2005) - a well-tested industry standard.
// Hardcoding this value avoids unnecessary configuration while maintaining compatibility.
const (
	binarySampleSize = 8000
	utf16BOMLen      = 2
	utf32BOMLen      = 4
)

// IsBinaryContent checks if content bytes contain binary data by looking for null bytes.
// It handles UTF-16 and UTF-32 BOMs specially to avoid false positives.
// This is a pure function with no state - import and use directly.
func IsBinaryContent(content []byte) bool {
	// Check for common text file BOMs (UTF-16, UTF-32)
	if len(content) >= utf16BOMLen {
		if (content[0] == 0xFF && content[1] == 0xFE) ||
			(content[0] == 0xFE && content[1] == 0xFF) {
			return false // UTF-16 BOM - treat as text, skip null check
		}
	}
	if len(content) >= utf32BOMLen {
		if (content[0] == 0xFF && content[1] == 0xFE && content[2] == 0x00 && content[3] == 0x00) ||
			(content[0] == 0x00 && content[1] == 0x00 && content[2] == 0xFE && content[3] == 0xFF) {
			return false // UTF-32 BOM - treat as text, skip null check
		}
	}

	// Check for null bytes in sample
	sampleSize := min(len(content), binarySampleSize)
	for i := range sampleSize {
		if content[i] == 0 {
			return true
		}
	}
	return false
}
