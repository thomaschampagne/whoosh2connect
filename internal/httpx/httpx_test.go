package httpx

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadBody(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"small body", "ok", false},
		{"exactly the limit", strings.Repeat("a", MaxBodyBytes), false},
		{"one byte over the limit", strings.Repeat("a", MaxBodyBytes+1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadBody(bytes.NewReader([]byte(tt.input)))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && string(got) != tt.input {
				t.Fatal("body mismatch")
			}
		})
	}
}

func TestSnippet(t *testing.T) {
	long := []byte(strings.Repeat("x", 500))
	if got := Snippet(long); len(got) != snippetLen {
		t.Errorf("len = %d, want %d", len(got), snippetLen)
	}
	if got := Snippet([]byte("short")); got != "short" {
		t.Errorf("got %q", got)
	}
}
