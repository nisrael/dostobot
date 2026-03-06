package main

import (
	"io"
)

// copyBuf copies from r to w using a buffer.
func copyBuf(w io.Writer, r io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	return io.CopyBuffer(w, r, buf)
}
