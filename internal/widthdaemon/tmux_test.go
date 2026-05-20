package widthdaemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeRunner is a CommandRunner that returns canned output (or a canned
// error) so we can exercise TmuxSource.Detect without forking tmux.
type fakeRunner struct {
	out []byte
	err error
}

func (f *fakeRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return f.out, f.err
}

// fakeExitError satisfies the same `ExitCode() int` interface that
// *exec.ExitError satisfies (via its embedded *os.ProcessState),
// without requiring us to actually run a subprocess in the test.
type fakeExitError struct{ code int }

func (e *fakeExitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e *fakeExitError) ExitCode() int { return e.code }

// The tmux source's format string is:
//
//	#{client_tty} #{client_width} #{client_session} #{client_activity}
//
// where client_activity is a Unix epoch second. Tests construct inputs
// directly in that shape.

func TestTmuxSource_Detect_SingleClient(t *testing.T) {
	src := &TmuxSource{Runner: &fakeRunner{out: []byte("/dev/pts/3 200 main 1779307440\n")}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Detect: want 1 source, got %d (%+v)", len(got), got)
	}
	want := Source{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 200, Session: "main"}
	if got[0] != want {
		t.Errorf("Detect: got %+v, want %+v", got[0], want)
	}
}

func TestTmuxSource_Detect_PicksMostRecentClient(t *testing.T) {
	// Two clients; second has higher activity timestamp → should win.
	out := "/dev/pts/3 200 main 1779307400\n/dev/pts/5 80 mobile 1779307500\n"
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Detect: want 1 source (the most recent), got %d (%+v)", len(got), got)
	}
	if got[0].TTY != "/dev/pts/5" {
		t.Errorf("Detect: should pick pts/5 (higher activity), got %s", got[0].TTY)
	}
	if got[0].Width != 80 {
		t.Errorf("Detect: width should be 80 (from pts/5), got %d", got[0].Width)
	}
}

func TestTmuxSource_Detect_IgnoresStaleClient(t *testing.T) {
	// Mirrors the production bug: phone idle for hours at 50 cols
	// alongside an active laptop at 254. The active one must win.
	out := "/dev/pts/6 50 mars 1779296796\n" + // ancient — phone
		"/dev/pts/0 254 earth 1779307440\n" // recent — laptop
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 || got[0].Width != 254 {
		t.Errorf("Detect: should pick the recent 254-col client, got %+v", got)
	}
}

func TestTmuxSource_Detect_NoServerExitCode1(t *testing.T) {
	src := &TmuxSource{Runner: &fakeRunner{err: &fakeExitError{code: 1}}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: want nil error for tmux-not-running, got %v", err)
	}
	if got != nil {
		t.Errorf("Detect: want nil slice for tmux-not-running, got %+v", got)
	}
}

func TestTmuxSource_Detect_OtherExitCodesAreErrors(t *testing.T) {
	src := &TmuxSource{Runner: &fakeRunner{err: &fakeExitError{code: 2}}}

	_, err := src.Detect(context.Background())
	if err == nil {
		t.Fatalf("Detect: want error for tmux exit 2, got nil")
	}
}

func TestTmuxSource_Detect_MalformedLineSkipped(t *testing.T) {
	out := "/dev/pts/3 200 main 1779307400\nthis is garbage\n/dev/pts/5 80 mobile 1779307500\n"
	var logBuf bytes.Buffer
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}, LogWriter: &logBuf}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	// Garbage line dropped, two valid clients parsed, most-recent picked.
	if len(got) != 1 || got[0].TTY != "/dev/pts/5" {
		t.Fatalf("Detect: want pts/5 (the most-recent valid client), got %+v", got)
	}
	if !strings.Contains(logBuf.String(), "garbage") {
		t.Errorf("Detect: malformed line should be logged; log was %q", logBuf.String())
	}
}

func TestTmuxSource_Detect_ZeroWidthDropped(t *testing.T) {
	out := "/dev/pts/3 0 main 1779307500\n/dev/pts/5 80 mobile 1779307400\n"
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	// Width-0 client dropped before activity comparison; pts/5 is the
	// only remaining valid client and wins by default.
	if len(got) != 1 || got[0].Width != 80 {
		t.Errorf("Detect: want only width=80 source, got %+v", got)
	}
}

func TestTmuxSource_Detect_NegativeWidthDropped(t *testing.T) {
	out := "/dev/pts/3 -5 main 1779307500\n/dev/pts/5 80 mobile 1779307400\n"
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 || got[0].Width != 80 {
		t.Errorf("Detect: want only width=80 source, got %+v", got)
	}
}

func TestTmuxSource_Detect_UnparseableActivityLogged(t *testing.T) {
	out := "/dev/pts/3 200 main not-a-number\n"
	var logBuf bytes.Buffer
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}, LogWriter: &logBuf}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != nil {
		t.Errorf("Detect: line with bad activity should be dropped, got %+v", got)
	}
	if !strings.Contains(logBuf.String(), "activity") {
		t.Errorf("Detect: bad activity should be logged; log was %q", logBuf.String())
	}
}

func TestTmuxSource_Detect_GenericRunnerErrorWrapped(t *testing.T) {
	boom := errors.New("boom")
	src := &TmuxSource{Runner: &fakeRunner{err: boom}}

	_, err := src.Detect(context.Background())
	if err == nil {
		t.Fatalf("Detect: want error for generic runner failure, got nil")
	}
	if !errors.Is(err, boom) {
		t.Errorf("Detect: error %q should wrap %q via errors.Is", err, boom)
	}
}

func TestTmuxSource_Detect_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	src := &TmuxSource{Runner: &fakeRunner{out: []byte("/dev/pts/3 200 main 1779307440\n")}}
	_, err := src.Detect(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Detect: want context.Canceled, got %v", err)
	}
}

func TestTmuxSource_Detect_EmptyOutput(t *testing.T) {
	src := &TmuxSource{Runner: &fakeRunner{out: []byte("")}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if got != nil {
		t.Errorf("Detect: want nil slice for empty output, got %+v", got)
	}
}

func TestTmuxSource_Detect_TiePreservesFirstSeen(t *testing.T) {
	// When two clients share the exact same activity timestamp, the
	// implementation picks whichever appeared first (Go's > comparison
	// in a single pass). Pinning this so the behavior doesn't drift.
	out := "/dev/pts/3 200 main 1779307440\n/dev/pts/5 80 mobile 1779307440\n"
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 || got[0].TTY != "/dev/pts/3" {
		t.Errorf("Detect: tie should resolve to first-seen pts/3, got %+v", got)
	}
}
