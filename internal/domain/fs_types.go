package domain

import (
	"io"
	"os"
)

// File defines a generic file handle for reading and seeking.
type File interface {
	io.Reader
	io.Seeker
	io.Closer
	Stat() (os.FileInfo, error)
}
