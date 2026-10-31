// Package httpx holds bounded HTTP response helpers shared by the API clients.
package httpx

import (
	"fmt"
	"io"
)

// MaxBodyBytes caps any single response body read into memory.
const MaxBodyBytes = 10 << 20

// snippetLen limits how much of an error body reaches an error message.
const snippetLen = 120

// ReadBody reads a response body up to MaxBodyBytes. A longer body is an error,
// so a misbehaving server cannot exhaust memory.
func ReadBody(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(data) > MaxBodyBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", MaxBodyBytes)
	}
	return data, nil
}

// Snippet returns a short, single-line prefix of an error body for messages.
func Snippet(b []byte) string {
	if len(b) > snippetLen {
		b = b[:snippetLen]
	}
	return string(b)
}
