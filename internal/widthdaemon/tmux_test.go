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

func TestTmuxSource_Detect_SingleClient(t *testing.T) {
	src := &TmuxSource{Runner: &fakeRunner{out: []byte("/dev/pts/3 200 main\n")}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Detect: want 1 source, got %d (%+v)", len(got), got)
	}
	want := Source{Kind: "tmux", TTY: "/dev/pts/3", Width: 200, Session: "main"}
	if got[0] != want {
		t.Errorf("Detect: got %+v, want %+v", got[0], want)
	}
}

func TestTmuxSource_Detect_MultipleClients(t *testing.T) {
	out := "/dev/pts/3 200 main\n/dev/pts/5 80 mobile\n"
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Detect: want 2 sources, got %d (%+v)", len(got), got)
	}
	if got[0].Width != 200 || got[1].Width != 80 {
		t.Errorf("Detect: widths = %d, %d; want 200, 80", got[0].Width, got[1].Width)
	}
	if got[0].Session != "main" || got[1].Session != "mobile" {
		t.Errorf("Detect: sessions = %q, %q; want main, mobile", got[0].Session, got[1].Session)
	}
}

func TestTmuxSource_Detect_NoServerExitCode1(t *testing.T) {
	// tmux exits with status 1 when no server is running.
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
	// Exit code 2+ from tmux is a real failure (bad args, etc.) and must propagate.
	src := &TmuxSource{Runner: &fakeRunner{err: &fakeExitError{code: 2}}}

	_, err := src.Detect(context.Background())
	if err == nil {
		t.Fatalf("Detect: want error for tmux exit 2, got nil")
	}
}

func TestTmuxSource_Detect_MalformedLineSkipped(t *testing.T) {
	out := "/dev/pts/3 200 main\nthis is garbage\n/dev/pts/5 80 mobile\n"
	var logBuf bytes.Buffer
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}, LogWriter: &logBuf}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Detect: want 2 sources (good lines), got %d (%+v)", len(got), got)
	}
	if !strings.Contains(logBuf.String(), "garbage") {
		t.Errorf("Detect: malformed line should be logged; log was %q", logBuf.String())
	}
}

func TestTmuxSource_Detect_ZeroWidthDropped(t *testing.T) {
	out := "/dev/pts/3 0 main\n/dev/pts/5 80 mobile\n"
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 || got[0].Width != 80 {
		t.Errorf("Detect: want only width=80 source, got %+v", got)
	}
}

func TestTmuxSource_Detect_NegativeWidthDropped(t *testing.T) {
	out := "/dev/pts/3 -5 main\n/dev/pts/5 80 mobile\n"
	src := &TmuxSource{Runner: &fakeRunner{out: []byte(out)}}

	got, err := src.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected error %v", err)
	}
	if len(got) != 1 || got[0].Width != 80 {
		t.Errorf("Detect: want only width=80 source, got %+v", got)
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
	cancel() // pre-cancel

	src := &TmuxSource{Runner: &fakeRunner{out: []byte("/dev/pts/3 200 main\n")}}
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
	if len(got) != 0 {
		t.Errorf("Detect: want empty slice for empty output, got %+v", got)
	}
}
