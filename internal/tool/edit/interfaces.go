package edit

// pathResolver defines workspace path resolution operations.
type pathResolver interface {
	ValidateAbs(path string) (string, error)
	DisplayPath(path string) string
}
