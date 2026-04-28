package content

const crlfLen = 2

// SplitLines splits content into lines, handling both \n and \r\n line endings.
// It returns a slice of strings, each representing a line without its line ending.
// If the content ends with a newline sequence, it does NOT return a trailing empty string.
func SplitLines(content string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, content[start:i])
			start = i + 1
		} else if content[i] == '\r' && i+1 < len(content) && content[i+1] == '\n' {
			lines = append(lines, content[start:i])
			start = i + crlfLen
			i++ // Skip the \n
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}
