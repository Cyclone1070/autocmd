// Package follow provides utilities for following real-time updates to a file.
package follow

import (
	"errors"
	"io"
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
)

type fileSystem interface {
	Open(path string) (domain.File, error)
}

// Follower implements io.Reader to perform "tail -f" logic on a file path.
// It is thread-safe and can be stopped gracefully.
type Follower struct {
	fs       fileSystem
	file     domain.File
	stopChan chan struct{}
	pokeChan chan struct{}
	path     string
	once     sync.Once
	mu       sync.Mutex
}

// NewFollower creates a new Follower for the given path.
func NewFollower(fs fileSystem, path string) *Follower {
	return &Follower{
		fs:       fs,
		path:     path,
		stopChan: make(chan struct{}),
		pokeChan: make(chan struct{}, 1),
	}
}

// Poke signals the follower that new data is likely available.

// Poke signals the follower that new data is likely available.
func (f *Follower) Poke() {
	select {
	case f.pokeChan <- struct{}{}:
	default:
	}
}

// Stop signals the reader to perform one final sweep and then return io.EOF.
func (f *Follower) Stop() {
	f.once.Do(func() {
		close(f.stopChan)
	})
}

func (f *Follower) Read(p []byte) (int, error) {
	if f.path == "" {
		return 0, io.EOF
	}

	for {
		// 2. Ensure file is open
		f.mu.Lock()
		if f.file == nil {
			var err error
			f.file, err = f.fs.Open(f.path)
			if err != nil {
				f.mu.Unlock()
				return 0, err
			}
		}
		file := f.file
		f.mu.Unlock()

		// 3. Try to read
		n, err := file.Read(p)
		if n > 0 {
			return n, nil
		}

		// 4. Handle EOF - wait for more data OR stop
		if errors.Is(err, io.EOF) {
			select {
			case <-f.pokeChan:
				// Data likely arrived, loop back and read immediately
				continue
			case <-f.stopChan:
				// Final check for any last-second data
				n, _ = file.Read(p)
				_ = file.Close()
				return n, io.EOF
			}
		}

		if err != nil {
			return n, err
		}
	}
}
