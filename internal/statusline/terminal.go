package statusline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// Width cache shared with the widthdaemon. These are vars (not
// const) so tests can override them without an interface seam.
//
//nolint:gochecknoglobals // intentional test seam; never written outside tests
var (
	widthCacheFile       = "/dev/shm/cc-tools/parent-width"
	widthCacheStaleAfter = 5 * time.Minute
)

// DefaultTerminalWidth provides terminal width detection.
type DefaultTerminalWidth struct{}

// GetWidth returns the current terminal width.
func (t *DefaultTerminalWidth) GetWidth() int {
	// Try various methods in priority order
	widthMethods := []func() int{
		t.getTestOverride,
		t.getColumnsEnv,
		t.getTmuxIfAvailable,
		t.getFromStderr,
		t.getFromStdout,
		t.getFromStdin,
		t.getFromTTY,
		t.getSSHWidth,
		t.getFromAncestorTTY,
		t.getWidthCache,
		getTputWidth,
		getSttyWidth,
	}

	for _, method := range widthMethods {
		if width := method(); width > 0 {
			return width
		}
	}

	// Default fallback
	return t.getDefault()
}

func (t *DefaultTerminalWidth) getColumnsEnv() int {
	if columns := os.Getenv("COLUMNS"); columns != "" {
		if width, err := strconv.Atoi(columns); err == nil && width > 0 {
			return width
		}
	}
	return 0
}

func (t *DefaultTerminalWidth) getTmuxIfAvailable() int {
	if tmux := os.Getenv("TMUX"); tmux != "" {
		return getTmuxWidth()
	}
	return 0
}

func (t *DefaultTerminalWidth) getFromStderr() int {
	if width, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && width > 0 {
		return width
	}
	return 0
}

func (t *DefaultTerminalWidth) getFromStdout() int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	return 0
}

func (t *DefaultTerminalWidth) getFromStdin() int {
	if width, _, err := term.GetSize(int(os.Stdin.Fd())); err == nil && width > 0 {
		return width
	}
	return 0
}

func (t *DefaultTerminalWidth) getFromTTY() int {
	if tty, err := os.Open("/dev/tty"); err == nil {
		defer func() { _ = tty.Close() }()
		if width, _, sizeErr := term.GetSize(int(tty.Fd())); sizeErr == nil && width > 0 {
			return width
		}
	}
	return 0
}

func (t *DefaultTerminalWidth) getDefault() int {
	const defaultWidth = 200
	if os.Getenv("DEBUG_WIDTH") == "1" {
		fmt.Fprintf(os.Stderr, "Using default width: %d\n", defaultWidth)
	}
	return defaultWidth
}

func (t *DefaultTerminalWidth) getTestOverride() int {
	if testWidth := os.Getenv("CLAUDE_STATUSLINE_WIDTH"); testWidth != "" {
		if width, err := strconv.Atoi(testWidth); err == nil && width > 0 {
			if os.Getenv("DEBUG_WIDTH") == "1" {
				fmt.Fprintf(os.Stderr, "Using CLAUDE_STATUSLINE_WIDTH: %d\n", width)
			}
			return width
		}
	}
	return 0
}

// getWidthCache reads /dev/shm/cc-tools/parent-width, which the
// widthdaemon updates on every detected change. Returns 0 when:
//   - the file doesn't exist (daemon not running)
//   - the file mtime is older than widthCacheStaleAfter (daemon dead)
//   - the contents don't parse as a positive integer
//
// Positioned in the priority list after the real TTY methods so a
// process that *does* have a TTY uses its own size; the cache is only
// the source of truth for the headless dispatched-agent case.
func (t *DefaultTerminalWidth) getWidthCache() int {
	info, err := os.Stat(widthCacheFile)
	if err != nil {
		return 0
	}
	if time.Since(info.ModTime()) > widthCacheStaleAfter {
		if os.Getenv("DEBUG_WIDTH") == "1" {
			fmt.Fprintf(os.Stderr, "Cache stale (mtime=%s): ignoring\n", info.ModTime())
		}
		return 0
	}
	data, err := os.ReadFile(widthCacheFile)
	if err != nil {
		return 0
	}
	width, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || width <= 0 {
		return 0
	}
	if os.Getenv("DEBUG_WIDTH") == "1" {
		fmt.Fprintf(os.Stderr, "Using cache width: %d\n", width)
	}
	return width
}

func (t *DefaultTerminalWidth) getSSHWidth() int {
	if sshTty := os.Getenv("SSH_TTY"); sshTty != "" {
		if file, err := os.Open(sshTty); err == nil { //nolint:gosec // SSH_TTY is a trusted env var
			defer func() { _ = file.Close() }()
			if width, _, sizeErr := term.GetSize(int(file.Fd())); sizeErr == nil && width > 0 {
				return width
			}
		}
	}
	return 0
}

// getFromAncestorTTY walks up the process tree from the parent pid looking for
// an ancestor whose controlling tty is a real pty device, then returns that
// pty's width. Recovers width when Claude Code spawns the statusline command
// detached from the terminal (all stdio piped, no controlling tty) but a
// shell or claude process higher up still owns the user's pty.
//
// Approach borrowed from moond4rk/ccstatus (Apache-2.0).
func (t *DefaultTerminalWidth) getFromAncestorTTY() int {
	const maxParentWalk = 8
	pid := os.Getppid()
	for range maxParentWalk {
		if pid <= 1 {
			break
		}
		ppid, tty := ppidAndTTY(pid)
		if width := widthFromTTYName(tty); width > 0 {
			return width
		}
		if ppid <= 1 || ppid == pid {
			break
		}
		pid = ppid
	}
	return 0
}

// ppidAndTTY reports a process's parent pid and controlling tty name via `ps`.
// `ps -o ppid=,tty=` is portable across Linux (returns e.g. "pts/3") and
// macOS (returns e.g. "ttys003"). Returns (0, "") on lookup failure.
func ppidAndTTY(pid int) (int, string) {
	const psTimeout = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()
	//nolint:gosec // pid is a sanitized integer, not user input
	cmd := exec.CommandContext(ctx, "ps", "-o", "ppid=,tty=", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return 0, ""
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return 0, ""
	}
	ppid, _ := strconv.Atoi(fields[0])
	var tty string
	if len(fields) > 1 {
		tty = fields[1]
	}
	return ppid, tty
}

// widthFromTTYName resolves a `ps`-style tty name (e.g. "ttys003" on macOS or
// "pts/3" on Linux) to a device path and returns its width, or 0 if the name
// is not a real terminal.
func widthFromTTYName(name string) int {
	name = strings.TrimSpace(name)
	switch name {
	case "", "?", "??", "-":
		return 0
	}
	path := name
	if !strings.HasPrefix(path, "/") {
		path = "/dev/" + path
	}
	file, err := os.Open(path) //nolint:gosec // path derived from ps output for our own session
	if err != nil {
		return 0
	}
	defer func() { _ = file.Close() }()
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil {
		return 0
	}
	return width
}

func getTmuxWidth() int {
	const commandTimeout = 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#{window_width}")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	width, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0
	}

	return width
}

func getTputWidth() int {
	const commandTimeout = 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tput", "cols")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	width, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0
	}

	return width
}

func getSttyWidth() int {
	const commandTimeout = 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "stty", "size")
	cmd.Stdin = os.Stdin // Important for stty to work
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	// stty size returns "rows cols"
	const expectedParts = 2
	parts := strings.Fields(string(output))
	if len(parts) != expectedParts {
		return 0
	}

	width, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}

	return width
}
