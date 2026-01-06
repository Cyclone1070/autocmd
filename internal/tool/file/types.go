package file

// -- Write File --

type WriteFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
