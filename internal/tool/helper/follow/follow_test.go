package follow

import (
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

type mockFS struct {
	mu      sync.Mutex
	content string
}

func (m *mockFS) Open(path string) (domain.File, error) {
	return &mockFile{fs: m, offset: 0}, nil
}

type mockFile struct {
	fs     *mockFS
	offset int
}

func (m *mockFile) Read(p []byte) (int, error) {
	m.fs.mu.Lock()
	defer m.fs.mu.Unlock()
	content := m.fs.content

	if m.offset >= len(content) {
		return 0, io.EOF
	}

	n := copy(p, content[m.offset:])
	m.offset += n
	return n, nil
}

func (m *mockFile) Close() error               { return nil }
func (m *mockFile) Stat() (os.FileInfo, error) { return nil, nil }
func (m *mockFile) Seek(offset int64, whence int) (int64, error) { return 0, nil }

func TestFollower_PokeLatency(t *testing.T) {
	fs := &mockFS{content: "initial"}
	f := NewFollower(fs, "test.log")

	// Consume initial
	buf := make([]byte, 100)
	n, err := f.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 7, n)

	// Start reading for new data in background
	readDone := make(chan int)
	go func() {
		n2, _ := f.Read(buf)
		readDone <- n2
	}()

	// Wait a bit to ensure it is blocked on EOF
	time.Sleep(50 * time.Millisecond)

	fs.mu.Lock()
	fs.content = "initialpoked"
	fs.mu.Unlock()

	start := time.Now()
	f.Poke()

	nAfter := <-readDone
	latency := time.Since(start)

	assert.Equal(t, 5, nAfter)
	assert.Less(t, latency, 50*time.Millisecond, "Latency too high! Poke() might be blocked or polling.")
}

