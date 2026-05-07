package dispatch

import (
	"bytes"
	"fmt"
)

const maxStdoutBytes = 10 * 1024 * 1024

type outputLimitError struct {
	stream string
	limit  int
}

func (e outputLimitError) Error() string {
	return fmt.Sprintf("plugin %s exceeded max size %d bytes", e.stream, e.limit)
}

type boundedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newBoundedBuffer(limit int) *boundedBuffer {
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) <= remaining {
		_, _ = b.buf.Write(p)
		return len(p), nil
	}
	_, _ = b.buf.Write(p[:remaining])
	b.truncated = true
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.buf.Bytes()
}

func (b *boundedBuffer) String() string {
	if b == nil {
		return ""
	}
	return b.buf.String()
}

func (b *boundedBuffer) Truncated() bool {
	return b != nil && b.truncated
}
