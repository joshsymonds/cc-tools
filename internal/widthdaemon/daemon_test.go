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

// runDaemon spins the loop in a goroutine, waits until both detectors
// have been polled minTicks times each (deterministic — driven by the
// detectors' call counters, not wall-clock sleep), then cancels and
// asserts a clean shutdown within 1s.
func runDaemon(t *testing.T, d *Daemon, tmux, utmp *fakeDetector, minTicks int32) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	// Spin until both detectors have observed at least minTicks calls.
	// 2s hard cap protects against a hung daemon.
	deadline := time.Now().Add(2 * time.Second)
	for tmux.calls.Load() < minTicks || utmp.calls.Load() < minTicks {
		if time.Now().After(deadline) {
			t.Fatalf("detectors did not reach %d ticks within 2s (tmux=%d utmp=%d)",
				minTicks, tmux.calls.Load(), utmp.calls.Load())
		}
		// 1ms is below the detectors' active interval in tests (5ms),
		// so we sample frequently without busy-spinning.
		time.Sleep(time.Millisecond)
	}

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
	runDaemon(t, d, tmux, utmp, 3)

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
	runDaemon(t, d, tmux, utmp, 8)

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
	runDaemon(t, d, tmux, utmp, 8)

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
	runDaemon(t, d, tmux, utmp, 3)

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
	runDaemon(t, d, tmux, utmp, 3)

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

// TestDaemon_Run_IdleIntervalEngagesAfterStability observes the
// behavior of the idle-interval transition. With an Active cadence
// much faster than Idle and IdleAfter elapsing during the run window,
// the *observed* tick rate must slow once IdleAfter has elapsed —
// otherwise the idle-branch in Run() is broken.
//
// This replaces a previous tautological default-mirror test (which
// only asserted that `Build` copied the constants onto the cfg
// struct) with a behavioral check.
func TestDaemon_Run_IdleIntervalEngagesAfterStability(t *testing.T) {
	tmux := &fakeDetector{results: [][]Source{
		{{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 80, Session: "main"}},
	}}
	utmp := &fakeDetector{results: [][]Source{nil}}
	sink := &fakeSink{}

	d := Build(Config{
		ActiveInterval: 2 * time.Millisecond,
		IdleInterval:   40 * time.Millisecond, // 20x slower
		IdleAfter:      20 * time.Millisecond,
		Tmux:           tmux,
		Utmp:           utmp,
		Sink:           sink,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	// Phase 1: wait until IdleAfter has demonstrably elapsed since
	// the first (and only) width change. At 2ms cadence with
	// IdleAfter=20ms, ~10 active ticks will pass before the idle
	// branch starts setting the timer to IdleInterval. We wait for
	// ~30ms to ensure we're past the transition and the *next* timer
	// reset will use IdleInterval.
	time.Sleep(35 * time.Millisecond)

	// Phase 2: now measure tick rate. Over a 120ms window at
	// IdleInterval=40ms we expect ~3 ticks. At ActiveInterval=2ms
	// we'd see ~60. The gap is wide enough that the test is robust
	// to scheduler jitter.
	ticksAtIdleStart := tmux.calls.Load()
	time.Sleep(120 * time.Millisecond)
	idleDelta := tmux.calls.Load() - ticksAtIdleStart

	cancel()
	<-done

	// 120ms / 40ms idle ≈ 3 idle ticks. If still in active mode
	// at 2ms cadence we'd see ~60. Pick an upper bound that catches
	// "idle never engages" but tolerates scheduler delay.
	const maxIdleTicks int32 = 10
	if idleDelta > maxIdleTicks {
		t.Errorf("expected idle backoff to slow ticks; saw %d ticks in 120ms after stability "+
			"(at IdleInterval=40ms this should be ~3, at ActiveInterval=2ms it would be ~60). "+
			"Idle branch likely not engaging.",
			idleDelta)
	}
	if idleDelta == 0 {
		t.Errorf("expected at least one idle tick in 120ms, got 0 — daemon may be stuck")
	}
}
