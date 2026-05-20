// Package widthdaemon implements terminal-width detection for headless
// Claude Code subagent contexts. The daemon polls tmux clients and
// non-tmux TTYs (via utmp + TIOCGWINSZ) and writes the canonical min
// width to a cache file that the statusline reads when no real TTY is
// available.
package widthdaemon

// Source is one detected terminal width observation. The daemon
// aggregates a slice of these into the canonical cached width.
//
// Kind is "tmux" for clients reported by `tmux list-clients`, and
// "tty" for raw PTY entries discovered via utmp.
//
// TTY is the device path (e.g. /dev/pts/3) and is used to de-duplicate
// utmp entries whose PTY is already owned by a tmux client.
//
// Session is only populated for tmux clients; it carries the tmux
// session name and exists for debugging in widths.json. It is unused
// for aggregation.
type Source struct {
	Kind    string `json:"kind"`
	TTY     string `json:"tty"`
	Width   int    `json:"width"`
	Session string `json:"session,omitempty"`
}
