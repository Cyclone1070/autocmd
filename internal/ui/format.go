package ui

import "fmt"

const (
	millionThreshold = 1_000_000
	thousand         = 1_000
)

func ShortNum(n int) string {
	switch {
	case n >= millionThreshold:
		return fmt.Sprintf("%.1fM", float64(n)/millionThreshold)
	case n >= thousand:
		return fmt.Sprintf("%dk", n/thousand)
	default:
		return fmt.Sprintf("%d", n)
	}
}
