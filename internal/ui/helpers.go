package ui

import "fmt"

func formatError(header string, err string, theme *theme) string {
	return fmt.Sprintf("%s — %s", header, theme.Error(err))
}
