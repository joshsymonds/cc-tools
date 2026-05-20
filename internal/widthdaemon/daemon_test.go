package widthdaemon

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDetector returns canned source slices in order. After exhausting
// the canned slice it returns the last entry on every call so tests
// can assert "stays stable from here on" behavior.
type fakeDetector struct {
	mu      sync.Mutex
	results [][]Source
	idx     int
	err     error
	calls   atomic.Int32
}

func (f *fakeDetector) Detect(ctx context.Context) ([]Source, error) {
	f.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.results) == 0 {
		return nil, nil
	}
	out := f.results[f.idx]
	if f.idx < len(f.results)-1 {
		f.idx++
	}
	return out, nil
}

// fakeSink records (width, sources) tuples written by the daemon.
type fakeSink struct {
	mu     sync.Mutex
	writes []sinkCall
}

type sinkCall struct {
	width   int
	sources []Source
}

func (f *fakeSink) Write(width int, sources []Source) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, sinkCall{width: width, sources: append([]Source(nil), sources...)})
	return nil
}

func (f *fakeSink) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

func (f *fakeSink) lastWidth() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) == 0 {
		return -1
	}
	return f.writes[len(f.writes)-1].width
}

// runDaemon spins the loop in a goroutine and stops it after wait.
func runDaemon(t *testing.T, d *Daemon, wait time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	time.Sleep(wait)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("Run did not return within 1s of ctx cancel")
	}
}

func TestDaemon_Run_SingleTickWrites(t *testing.T) {
	tmux := &fakeDetector{results: [][]Source{
		{{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 80, Session: "main"}},
	}}
	utmp := &fakeDetector{results: [][]Source{nil}}
	sink := &fakeSink{}

	d := Build(Config{
		ActiveInterval: 5 * time.Millisecond,
		IdleInterval:   5 * time.Millisecond,
		IdleAfter:      100 * time.Millisecond,
		Tmux:           tmux,
		Utmp:           utmp,
		Sink:           sink,
	})
	runDaemon(t, d, 30*time.Millisecond)

	if sink.callCount() == 0 {
		t.Fatalf("expected at least one write, got none")
	}
	if sink.lastWidth() != 80 {
		t.Errorf("lastWidth = %d, want 80", sink.lastWidth())
	}
}

func TestDaemon_Run_StableWidthWritesOnce(t *testing.T) {
	tmux := &fakeDetector{results: [][]Source{
		{{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 80, Session: "main"}},
	}}
	utmp := &fakeDetector{results: [][]Source{nil}}
	sink := &fakeSink{}

	d := Build(Config{
		ActiveInterval: 5 * time.Millisecond,
		IdleInterval:   5 * time.Millisecond,
		IdleAfter:      100 * time.Millisecond,
		Tmux:           tmux,
		Utmp:           utmp,
		Sink:           sink,
	})
	runDaemon(t, d, 50*time.Millisecond)

	// Many ticks fired but only the first should have written, since
	// the width never changed.
	if got := sink.callCount(); got != 1 {
		t.Errorf("stable width should produce exactly 1 write, got %d", got)
	}
}

func TestDaemon_Run_WidthChangeTriggersWrite(t *testing.T) {
	tmux := &fakeDetector{results: [][]Source{
		{{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 80, Session: "main"}},
		{{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 200, Session: "main"}},
	}}
	utmp := &fakeDetector{results: [][]Source{nil}}
	sink := &fakeSink{}

	d := Build(Config{
		ActiveInterval: 5 * time.Millisecond,
		IdleInterval:   5 * time.Millisecond,
		IdleAfter:      100 * time.Millisecond,
		Tmux:           tmux,
		Utmp:           utmp,
		Sink:           sink,
	})
	runDaemon(t, d, 50*time.Millisecond)

	if got := sink.callCount(); got != 2 {
		t.Errorf("width change should produce 2 writes, got %d", got)
	}
	if sink.lastWidth() != 200 {
		t.Errorf("lastWidth = %d, want 200", sink.lastWidth())
	}
}

func TestDaemon_Run_AllSourcesEmptyNoWrite(t *testing.T) {
	tmux := &fakeDetector{results: [][]Source{nil}}
	utmp := &fakeDetector{results: [][]Source{nil}}
	sink := &fakeSink{}

	d := Build(Config{
		ActiveInterval: 5 * time.Millisecond,
		IdleInterval:   5 * time.Millisecond,
		IdleAfter:      100 * time.Millisecond,
		Tmux:           tmux,
		Utmp:           utmp,
		Sink:           sink,
	})
	runDaemon(t, d, 30*time.Millisecond)

	if got := sink.callCount(); got != 0 {
		t.Errorf("empty sources should produce 0 writes, got %d", got)
	}
}

func TestDaemon_Run_DetectorErrorContinues(t *testing.T) {
	tmux := &fakeDetector{err: errors.New("tmux exploded")}
	utmp := &fakeDetector{results: [][]Source{
		{{Kind: SourceKindTTY, TTY: "/dev/pts/5", Width: 80}},
	}}
	sink := &fakeSink{}

	d := Build(Config{
		ActiveInterval: 5 * time.Millisecond,
		IdleInterval:   5 * time.Millisecond,
		IdleAfter:      100 * time.Millisecond,
		Tmux:           tmux,
		Utmp:           utmp,
		Sink:           sink,
	})
	runDaemon(t, d, 30*time.Millisecond)

	// One source erroring shouldn't kill the daemon; the other source
	// should still produce a write.
	if got := sink.lastWidth(); got != 80 {
		t.Errorf("lastWidth = %d (want 80) — utmp width should survive tmux error", got)
	}
}

func TestDaemon_Run_ContextCancelReturnsPromptly(t *testing.T) {
	tmux := &fakeDetector{results: [][]Source{nil}}
	utmp := &fakeDetector{results: [][]Source{nil}}
	sink := &fakeSink{}

	d := Build(Config{
		ActiveInterval: 50 * time.Millisecond,
		IdleInterval:   50 * time.Millisecond,
		IdleAfter:      100 * time.Millisecond,
		Tmux:           tmux,
		Utmp:           utmp,
		Sink:           sink,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	// Cancel immediately; loop must return within one interval window.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Run did not return within 100ms of ctx cancel")
	}
}

func TestDaemon_Run_DefaultsApplied(t *testing.T) {
	d := Build(Config{Sink: &fakeSink{}, Tmux: &fakeDetector{}, Utmp: &fakeDetector{}})
	if d.cfg.ActiveInterval != defaultActiveInterval {
		t.Errorf("default ActiveInterval = %v, want %v", d.cfg.ActiveInterval, defaultActiveInterval)
	}
	if d.cfg.IdleInterval != defaultIdleInterval {
		t.Errorf("default IdleInterval = %v, want %v", d.cfg.IdleInterval, defaultIdleInterval)
	}
	if d.cfg.IdleAfter != defaultIdleAfter {
		t.Errorf("default IdleAfter = %v, want %v", d.cfg.IdleAfter, defaultIdleAfter)
	}
}
