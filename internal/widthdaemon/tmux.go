package widthdaemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// tmuxSubprocessTimeout caps how long we wait for tmux to respond. tmux
// is usually instant; this is just a safety net against a hung server.
const tmuxSubprocessTimeout = 2 * time.Second

// tmuxNotRunningExitCode is what `tmux list-clients` returns when no
// server is up. We treat it as a benign "no data" outcome rather than
// an error so the daemon doesn't spam the journal on every tick when
// tmux isn't in use.
const tmuxNotRunningExitCode = 1

// CommandRunner abstracts subprocess execution so tests can inject
// canned responses without forking real binaries. The context governs
// cancellation and timeout — implementations must honor it.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// DefaultCommandRunner runs commands via os/exec, honoring the context
// for cancellation and applying a built-in subprocess timeout.
type DefaultCommandRunner struct{}

// Run executes name with args under a context derived from ctx with a
// subprocess timeout applied. Stdout is returned; stderr is discarded
// (tmux puts errors there but we classify failures via exit code).
func (DefaultCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	subCtx, cancel := context.WithTimeout(ctx, tmuxSubprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(subCtx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", name, err)
	}
	return out, nil
}

// tmuxNotRunningCacheTTL is how long we trust a prior "tmux not
// running" result before re-probing. At the default daemon idle
// cadence of 5s that's a 6x reduction in subprocess forks for hosts
// where tmux is never used.
const tmuxNotRunningCacheTTL = 30 * time.Second

// TmuxSource detects terminal widths from attached tmux clients.
//
// Runner is injected so tests can supply fake subprocess output.
//
// LogWriter receives one-line warnings about malformed tmux output. It
// defaults to os.Stderr when nil, which means journalctl picks them up
// in the deployed daemon.
//
// notRunningUntil records when we may next try tmux after observing
// exit code 1 ("no server running"). Single-goroutine access from
// Daemon.Run; if multi-goroutine callers ever appear, this field
// needs a sync.Mutex.
type TmuxSource struct {
	Runner          CommandRunner
	LogWriter       io.Writer
	notRunningUntil time.Time
}

// Detect runs `tmux list-clients -F '#{client_tty} #{client_width}
// #{client_session}'` and parses each line into a Source. When tmux is
// not running (exit code 1 from list-clients), Detect returns
// (nil, nil) — absence of tmux is a valid state, not an error.
//
// Malformed lines are logged to LogWriter and skipped; other lines in
// the same response still parse. Widths <= 0 are dropped silently
// because they cannot contribute meaningfully to the aggregate min.
func (t *TmuxSource) Detect(ctx context.Context) ([]Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("tmux detect: %w", err)
	}

	if !t.notRunningUntil.IsZero() && time.Now().Before(t.notRunningUntil) {
		return nil, nil
	}

	out, err := t.Runner.Run(ctx, "tmux", "list-clients", "-F", "#{client_tty} #{client_width} #{client_session}")
	if err != nil {
		var exitCoder interface{ ExitCode() int }
		if errors.As(err, &exitCoder) && exitCoder.ExitCode() == tmuxNotRunningExitCode {
			t.notRunningUntil = time.Now().Add(tmuxNotRunningCacheTTL)
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-clients: %w", err)
	}
	// Successful probe — clear any stale negative cache.
	t.notRunningUntil = time.Time{}

	logTo := t.logWriter()

	lines := bytes.Split(out, []byte("\n"))
	sources := make([]Source, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(string(raw))
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		const expectedFields = 3
		if len(parts) != expectedFields {
			_, _ = fmt.Fprintf(logTo,
				"tmux: skipping malformed line %q (got %d fields, want %d)\n",
				line, len(parts), expectedFields)
			continue
		}
		width, parseErr := strconv.Atoi(parts[1])
		if parseErr != nil {
			_, _ = fmt.Fprintf(logTo, "tmux: skipping line with unparseable width %q: %v\n", line, parseErr)
			continue
		}
		if width <= 0 {
			continue
		}
		sources = append(sources, Source{
			Kind:    SourceKindTmux,
			TTY:     parts[0],
			Width:   width,
			Session: parts[2],
		})
	}
	return sources, nil
}

func (t *TmuxSource) logWriter() io.Writer {
	if t.LogWriter != nil {
		return t.LogWriter
	}
	return os.Stderr
}
