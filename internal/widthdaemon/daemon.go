package widthdaemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// Default cadence values. These are also the production defaults — the
// home-manager module override them when the user configures non-default
// intervals.
const (
	defaultActiveInterval = 1 * time.Second
	defaultIdleInterval   = 5 * time.Second
	defaultIdleAfter      = 30 * time.Second
)

// Detector is what each source (tmux, utmp) implements. The daemon
// loop drives them at every tick.
type Detector interface {
	Detect(ctx context.Context) ([]Source, error)
}

// Sink is what the daemon writes its aggregated result through. In
// production this is *Writer; tests can substitute a recording fake.
type Sink interface {
	Write(width int, sources []Source) error
}

// Config groups everything the daemon needs to run. All fields are
// optional and have sensible defaults except Tmux/Utmp/Sink, which
// must be supplied (the constructor wires production defaults when
// they're nil to keep the cmd/cc-tools wiring trivial).
type Config struct {
	// ActiveInterval is the polling cadence after a recent width
	// change. Defaults to 1s.
	ActiveInterval time.Duration

	// IdleInterval is the polling cadence once IdleAfter has elapsed
	// without a width change. Defaults to 5s.
	IdleInterval time.Duration

	// IdleAfter is how long we wait without a width change before
	// dropping to IdleInterval. Defaults to 30s.
	IdleAfter time.Duration

	// WriterDir is the directory where parent-width and widths.json
	// live. Defaults to /dev/shm/cc-tools.
	WriterDir string

	// Tmux, Utmp are the source detectors. When nil, production
	// defaults are wired in by New().
	Tmux Detector
	Utmp Detector

	// Sink receives the aggregated min width and the per-source slice
	// once per tick that produces a change. When nil, a *Writer
	// pointing at WriterDir is used.
	Sink Sink

	// LogWriter receives daemon-level warnings and per-tick stats.
	// Defaults to os.Stderr (captured by journald in deployment).
	LogWriter io.Writer
}

// Daemon is the long-running loop that polls sources, aggregates them
// into a canonical min width, and persists the result.
//
// Concurrency: all internal state is touched only from the single Run
// goroutine. Multiple Daemon instances may run in the same process,
// but Run must not be called concurrently on the same instance.
type Daemon struct {
	cfg    Config
	logger io.Writer
}

// New applies defaults and returns a ready-to-Run daemon.
func New(cfg Config) *Daemon {
	if cfg.ActiveInterval <= 0 {
		cfg.ActiveInterval = defaultActiveInterval
	}
	if cfg.IdleInterval <= 0 {
		cfg.IdleInterval = defaultIdleInterval
	}
	if cfg.IdleAfter <= 0 {
		cfg.IdleAfter = defaultIdleAfter
	}
	if cfg.Tmux == nil {
		cfg.Tmux = &TmuxSource{Runner: DefaultCommandRunner{}}
	}
	if cfg.Utmp == nil {
		cfg.Utmp = &UtmpSource{}
	}
	if cfg.Sink == nil {
		cfg.Sink = &Writer{Dir: cfg.WriterDir}
	}
	logger := cfg.LogWriter
	if logger == nil {
		logger = os.Stderr
	}
	return &Daemon{cfg: cfg, logger: logger}
}

// Run drives the polling loop until ctx is canceled, then returns nil.
// Detector errors are logged but never fatal — one bad source must not
// poison the others. The first tick fires immediately so a freshly
// started daemon doesn't leave the cache empty for a full interval.
func (d *Daemon) Run(ctx context.Context) error {
	var lastWidth int
	var lastChange time.Time
	interval := d.cfg.ActiveInterval

	// Fire immediately, then on a timer.
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}

		sources, width, ok := d.tick(ctx)

		if ok && width != lastWidth {
			if writeErr := d.cfg.Sink.Write(width, sources); writeErr != nil {
				_, _ = fmt.Fprintf(d.logger, "widthdaemon: write failed: %v\n", writeErr)
			} else {
				lastWidth = width
				lastChange = time.Now()
				interval = d.cfg.ActiveInterval
			}
		} else if !lastChange.IsZero() && time.Since(lastChange) >= d.cfg.IdleAfter {
			interval = d.cfg.IdleInterval
		}

		timer.Reset(interval)
	}
}

// tick runs one detection cycle. Returns the aggregated sources, the
// min width, and whether a real width was detected (false means leave
// the cache alone — epic anti-pattern: never write 0).
func (d *Daemon) tick(ctx context.Context) ([]Source, int, bool) {
	tmuxSources, tmuxErr := d.cfg.Tmux.Detect(ctx)
	if tmuxErr != nil && !errors.Is(tmuxErr, context.Canceled) {
		_, _ = fmt.Fprintf(d.logger, "widthdaemon: tmux detect: %v\n", tmuxErr)
	}
	utmpSources, utmpErr := d.cfg.Utmp.Detect(ctx)
	if utmpErr != nil && !errors.Is(utmpErr, context.Canceled) {
		_, _ = fmt.Fprintf(d.logger, "widthdaemon: utmp detect: %v\n", utmpErr)
	}

	combined := make([]Source, 0, len(tmuxSources)+len(utmpSources))
	combined = append(combined, tmuxSources...)
	combined = append(combined, utmpSources...)
	aggregated := Aggregate(combined)
	width, ok := MinWidth(aggregated)
	return aggregated, width, ok
}
