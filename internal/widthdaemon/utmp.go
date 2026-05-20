package widthdaemon

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Linux utmpx on-disk layout (glibc x86_64). Verified empirically:
// sizeof(struct utmpx) = 384, offsetof(ut_type) = 0,
// offsetof(ut_line) = 8, sizeof(ut_line) = 32.
const (
	utmpxRecordSize      = 384
	utmpxTypeOffset      = 0
	utmpxLineOffset      = 8
	utmpxLineSize        = 32
	utmpxTypeUserProcess = 7
	defaultUtmpPath      = "/var/run/utmp"
)

// WinsizeReader queries the window size of a TTY device. It is the
// boundary that lets tests substitute canned widths for real ioctls.
//
// Implementations MUST open the device with O_NOCTTY so the daemon
// never claims a controlling terminal — that's a hard requirement
// from the epic's anti-patterns.
type WinsizeReader interface {
	Read(ttyPath string) (cols uint16, err error)
}

// DefaultWinsizeReader queries TIOCGWINSZ on the live TTY device.
type DefaultWinsizeReader struct{}

// Read opens ttyPath with O_RDONLY|O_NOCTTY, queries TIOCGWINSZ, and
// returns the column count. O_NOCTTY is non-negotiable: without it,
// opening a TTY can promote it to the daemon's controlling terminal,
// which would let SIGHUP/SIGWINCH propagate to a process that has no
// business receiving them.
func (DefaultWinsizeReader) Read(ttyPath string) (uint16, error) {
	// gosec G304: ttyPath originates from /var/run/utmp (root-owned)
	// and is bounded to /dev/<ut_line> by the caller. Reading
	// arbitrary paths is the daemon's whole purpose.
	f, err := os.OpenFile(ttyPath, os.O_RDONLY|syscall.O_NOCTTY, 0) //nolint:gosec // see comment above
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", ttyPath, err)
	}
	defer func() { _ = f.Close() }()

	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, fmt.Errorf("TIOCGWINSZ %s: %w", ttyPath, err)
	}
	return ws.Col, nil
}

// UtmpSource detects terminal widths from active user sessions
// recorded in /var/run/utmp. For each USER_PROCESS entry it queries
// the device's current winsize via the injected WinsizeReader.
//
// Path defaults to /var/run/utmp when empty.
// WinsizeReader defaults to DefaultWinsizeReader{} when nil.
// LogWriter defaults to os.Stderr when nil.
type UtmpSource struct {
	Path          string
	WinsizeReader WinsizeReader
	LogWriter     io.Writer
}

// Detect parses utmp, queries winsize for each USER_PROCESS entry, and
// returns one Source per successfully-read TTY. Missing utmp returns
// (nil, nil) because systems without utmp are a valid state. Per-TTY
// failures are logged and skipped — one disappearing session must not
// poison the whole batch.
func (u *UtmpSource) Detect(ctx context.Context) ([]Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("utmp detect: %w", err)
	}

	path := u.Path
	if path == "" {
		path = defaultUtmpPath
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is daemon-controlled
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read utmp %s: %w", path, err)
	}

	reader := u.WinsizeReader
	if reader == nil {
		reader = DefaultWinsizeReader{}
	}
	logTo := u.LogWriter
	if logTo == nil {
		logTo = os.Stderr
	}

	recordCount := len(data) / utmpxRecordSize
	sources := make([]Source, 0, recordCount)
	for i := 0; i+utmpxRecordSize <= len(data); i += utmpxRecordSize {
		rec := data[i : i+utmpxRecordSize]
		// ut_type is technically a signed short in C, but we only care
		// whether it equals USER_PROCESS (7) — a positive small int —
		// so we can stay in unsigned arithmetic and dodge G115.
		utType := binary.LittleEndian.Uint16(rec[utmpxTypeOffset : utmpxTypeOffset+2])
		if utType != utmpxTypeUserProcess {
			continue
		}
		line := nullTerminatedString(rec[utmpxLineOffset : utmpxLineOffset+utmpxLineSize])
		if line == "" {
			continue
		}
		device := "/dev/" + line
		cols, wsErr := reader.Read(device)
		if wsErr != nil {
			_, _ = fmt.Fprintf(logTo, "utmp: skipping %s: %v\n", device, wsErr)
			continue
		}
		if cols == 0 {
			continue
		}
		sources = append(sources, Source{Kind: "tty", TTY: device, Width: int(cols)})
	}
	return sources, nil
}

func nullTerminatedString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
