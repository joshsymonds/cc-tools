package widthdaemon

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeWinsizeReader returns canned widths per path, or canned errors.
type fakeWinsizeReader struct {
	cols map[string]uint16
	errs map[string]error
}

func (f *fakeWinsizeReader) Read(ttyPath string) (uint16, error) {
	if err, ok := f.errs[ttyPath]; ok {
		return 0, err
	}
	return f.cols[ttyPath], nil
}

// writeUtmpRecord appends a single 384-byte utmpx-like record to buf.
// Only ut_type (offset 0, int16 LE) and ut_line (offset 8, 32 bytes
// null-terminated) are populated — the parser ignores everything else.
func writeUtmpRecord(t *testing.T, buf *bytes.Buffer, utType int16, line string) {
	t.Helper()
	rec := make([]byte, utmpxRecordSize)
	binary.LittleEndian.PutUint16(rec[utmpxTypeOffset:utmpxTypeOffset+2], uint16(utType))
	if len(line) > utmpxLineSize-1 {
		line = line[:utmpxLineSize-1]
	}
	copy(rec[utmpxLineOffset:utmpxLineOffset+utmpxLineSize], line)
	// Ensure null terminator — copy already pads with zeros past line end.
	buf.Write(rec)
}

func writeTempUtmp(t *testing.T, records func(*bytes.Buffer)) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "utmp")
	var buf bytes.Buffer
	records(&buf)
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write temp utmp: %v", err)
	}
	return path
}

func TestUtmpSource_Detect_EmptyFile(t *testing.T) {
	path := writeTempUtmp(t, func(_ *bytes.Buffer) {})
	src := &UtmpSource{Path: path, WinsizeReader: &fakeWinsizeReader{}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect: want empty slice, got %+v", got)
	}
}

func TestUtmpSource_Detect_MissingFile(t *testing.T) {
	src := &UtmpSource{Path: "/nonexistent/utmp", WinsizeReader: &fakeWinsizeReader{}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: want nil error for missing utmp, got %v", err)
	}
	if got != nil {
		t.Errorf("Detect: want nil slice for missing utmp, got %+v", got)
	}
}

func TestUtmpSource_Detect_SingleUserProcess(t *testing.T) {
	path := writeTempUtmp(t, func(b *bytes.Buffer) {
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "pts/3")
	})
	wsr := &fakeWinsizeReader{cols: map[string]uint16{"/dev/pts/3": 200}}
	src := &UtmpSource{Path: path, WinsizeReader: wsr}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Detect: want 1 source, got %d (%+v)", len(got), got)
	}
	want := Source{Kind: "tty", TTY: "/dev/pts/3", Width: 200}
	if got[0] != want {
		t.Errorf("Detect: got %+v, want %+v", got[0], want)
	}
}

func TestUtmpSource_Detect_MixedTypes(t *testing.T) {
	path := writeTempUtmp(t, func(b *bytes.Buffer) {
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "pts/3")
		writeUtmpRecord(t, b, 6 /* LOGIN_PROCESS */, "tty1")
		writeUtmpRecord(t, b, 8 /* DEAD_PROCESS */, "pts/9")
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "pts/5")
	})
	wsr := &fakeWinsizeReader{cols: map[string]uint16{
		"/dev/pts/3": 200,
		"/dev/pts/5": 80,
	}}
	src := &UtmpSource{Path: path, WinsizeReader: wsr}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Detect: want 2 USER_PROCESS sources, got %d (%+v)", len(got), got)
	}
}

func TestUtmpSource_Detect_WinsizeFailureSkipsThatTTY(t *testing.T) {
	path := writeTempUtmp(t, func(b *bytes.Buffer) {
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "pts/3")
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "pts/5")
	})
	wsr := &fakeWinsizeReader{
		cols: map[string]uint16{"/dev/pts/3": 0, "/dev/pts/5": 80}, // pts/3 errors
		errs: map[string]error{"/dev/pts/3": errors.New("device gone")},
	}
	var logBuf bytes.Buffer
	src := &UtmpSource{Path: path, WinsizeReader: wsr, LogWriter: &logBuf}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 || got[0].TTY != "/dev/pts/5" {
		t.Errorf("Detect: want only pts/5, got %+v", got)
	}
	if !strings.Contains(logBuf.String(), "pts/3") {
		t.Errorf("Detect: failure should be logged, log was %q", logBuf.String())
	}
}

func TestUtmpSource_Detect_ZeroWidthDropped(t *testing.T) {
	path := writeTempUtmp(t, func(b *bytes.Buffer) {
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "pts/3")
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "pts/5")
	})
	wsr := &fakeWinsizeReader{cols: map[string]uint16{"/dev/pts/3": 0, "/dev/pts/5": 80}}
	src := &UtmpSource{Path: path, WinsizeReader: wsr}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 || got[0].TTY != "/dev/pts/5" {
		t.Errorf("Detect: want only pts/5, got %+v", got)
	}
}

func TestUtmpSource_Detect_EmptyLineSkipped(t *testing.T) {
	path := writeTempUtmp(t, func(b *bytes.Buffer) {
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "") // empty ut_line
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "pts/5")
	})
	wsr := &fakeWinsizeReader{cols: map[string]uint16{"/dev/pts/5": 80}}
	src := &UtmpSource{Path: path, WinsizeReader: wsr}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 || got[0].TTY != "/dev/pts/5" {
		t.Errorf("Detect: want only pts/5, got %+v", got)
	}
}

func TestUtmpSource_Detect_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	path := writeTempUtmp(t, func(b *bytes.Buffer) {
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "pts/3")
	})
	src := &UtmpSource{Path: path, WinsizeReader: &fakeWinsizeReader{}}

	_, err := src.Detect(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Detect: want context.Canceled, got %v", err)
	}
}

func TestUtmpSource_Detect_PartialTrailingRecord(t *testing.T) {
	// A file whose size is not a multiple of 384 should not panic; we
	// parse the full records we can and ignore the trailing fragment.
	path := writeTempUtmp(t, func(b *bytes.Buffer) {
		writeUtmpRecord(t, b, utmpxTypeUserProcess, "pts/3")
		b.Write([]byte{0x07, 0x00, 0x00, 0x00}) // 4 stray bytes
	})
	wsr := &fakeWinsizeReader{cols: map[string]uint16{"/dev/pts/3": 200}}
	src := &UtmpSource{Path: path, WinsizeReader: wsr}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 {
		t.Errorf("Detect: want 1 source from the valid record, got %+v", got)
	}
}
