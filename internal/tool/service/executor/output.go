package executor

import (
	"bytes"

	"github.com/Cyclone1070/iav/internal/tool/helper/content"
)

// collector captures command output with size limits and binary content detection.
type collector struct {
	buffer    bytes.Buffer
	maxBytes  int
	truncated bool
	isBinary  bool

	bytesChecked int
	sampleSize   int
}

func newCollector(maxBytes int, sampleSize int) *collector {
	return &collector{
		maxBytes:   maxBytes,
		sampleSize: sampleSize,
	}
}

func (c *collector) Write(p []byte) (n int, err error) {
	if c.isBinary {
		return len(p), nil
	}

	if c.bytesChecked < c.sampleSize {
		remainingCheck := c.sampleSize - c.bytesChecked
		toCheck := p
		if len(toCheck) > remainingCheck {
			toCheck = toCheck[:remainingCheck]
		}

		if content.IsBinaryContent(toCheck) {
			c.isBinary = true
			c.truncated = true
			return len(p), nil
		}
		c.bytesChecked += len(toCheck)
	}

	remainingSpace := c.maxBytes - c.buffer.Len()
	if remainingSpace <= 0 {
		c.truncated = true
		return len(p), nil
	}

	toWrite := p
	if len(toWrite) > remainingSpace {
		toWrite = toWrite[:remainingSpace]
		c.truncated = true
	}

	written, err := c.buffer.Write(toWrite)
	if err != nil {
		return written, err
	}

	return len(p), nil
}

func (c *collector) String() string {
	if c.isBinary {
		return "[Binary Content]"
	}
	return c.buffer.String()
}

func (c *collector) Truncated() bool {
	return c.truncated
}

func (c *collector) Reset() {
	c.buffer.Reset()
	c.bytesChecked = 0
	c.isBinary = false
}
