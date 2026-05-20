package subagentstatusline

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestWriteDecorations_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteDecorations(&buf, nil); err != nil {
		t.Fatalf("WriteDecorations(nil): %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty decorations should write nothing, got %q", buf.String())
	}

	buf.Reset()
	if err := WriteDecorations(&buf, []Decoration{}); err != nil {
		t.Fatalf("WriteDecorations(empty): %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty slice should write nothing, got %q", buf.String())
	}
}

func TestWriteDecorations_Single(t *testing.T) {
	var buf bytes.Buffer
	in := []Decoration{{ID: "t1", Content: "hello"}}
	if err := WriteDecorations(&buf, in); err != nil {
		t.Fatalf("WriteDecorations: %v", err)
	}
	want := `{"id":"t1","content":"hello"}` + "\n"
	if buf.String() != want {
		t.Errorf("output = %q, want %q", buf.String(), want)
	}
}

func TestWriteDecorations_Multiple(t *testing.T) {
	var buf bytes.Buffer
	in := []Decoration{
		{ID: "t1", Content: "alpha"},
		{ID: "t2", Content: "beta"},
		{ID: "t3", Content: "gamma"},
	}
	if err := WriteDecorations(&buf, in); err != nil {
		t.Fatalf("WriteDecorations: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("output has %d lines, want 3:\n%s", len(lines), buf.String())
	}
	// Each line must parse independently.
	for i, line := range lines {
		var d Decoration
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Errorf("line %d %q not valid JSON: %v", i, line, err)
		}
	}
}

func TestWriteDecorations_ANSIContent(t *testing.T) {
	var buf bytes.Buffer
	ansi := "\033[38;2;180;190;254m\033[48;2;180;190;254m hi \033[0m"
	in := []Decoration{{ID: "t1", Content: ansi}}
	if err := WriteDecorations(&buf, in); err != nil {
		t.Fatalf("WriteDecorations: %v", err)
	}
	// Round trip through json.Unmarshal must recover the original content.
	var d Decoration
	if err := json.Unmarshal(bytes.TrimRight(buf.Bytes(), "\n"), &d); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if d.Content != ansi {
		t.Errorf("Content roundtrip mismatch:\n got %q\nwant %q", d.Content, ansi)
	}
}

func TestWriteDecorations_QuotesAndBackslashes(t *testing.T) {
	var buf bytes.Buffer
	tricky := `she said "hi \world"`
	in := []Decoration{{ID: "t1", Content: tricky}}
	if err := WriteDecorations(&buf, in); err != nil {
		t.Fatalf("WriteDecorations: %v", err)
	}
	var d Decoration
	if err := json.Unmarshal(bytes.TrimRight(buf.Bytes(), "\n"), &d); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if d.Content != tricky {
		t.Errorf("Content roundtrip mismatch:\n got %q\nwant %q", d.Content, tricky)
	}
}

// failingWriter fails on the Nth byte written.
type failingWriter struct {
	allowed int
	written int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.written >= f.allowed {
		return 0, errors.New("disk full")
	}
	n := len(p)
	if f.written+n > f.allowed {
		n = f.allowed - f.written
	}
	f.written += n
	if f.written >= f.allowed {
		return n, errors.New("disk full")
	}
	return n, nil
}

func TestWriteDecorations_WriteErrorPropagates(t *testing.T) {
	// Allow only a few bytes so the first decoration's write fails mid-stream.
	w := &failingWriter{allowed: 5}
	in := []Decoration{
		{ID: "t1", Content: "first"},
		{ID: "t2", Content: "second"},
	}
	err := WriteDecorations(w, in)
	if err == nil {
		t.Fatalf("WriteDecorations: want write error, got nil")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error %q should wrap underlying writer error", err)
	}
}
