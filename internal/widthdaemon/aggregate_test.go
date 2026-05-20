package widthdaemon

import (
	"reflect"
	"testing"
)

func TestAggregate_Empty(t *testing.T) {
	got := Aggregate(nil)
	if len(got) != 0 {
		t.Errorf("Aggregate(nil): want empty, got %+v", got)
	}
}

func TestAggregate_TmuxPresentDropsAllUtmp(t *testing.T) {
	// New rule: when tmux is reporting any client, utmp entries are
	// dropped entirely — even ones whose TTY tmux didn't return. The
	// motivating bug: tmux source picks the most-recent client and
	// returns just one, but utmp surfaces the OTHER PTYs (including a
	// stale phone PTY) which would drag MinWidth back down.
	in := []Source{
		{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 200, Session: "main"},
		{Kind: SourceKindTTY, TTY: "/dev/pts/5", Width: 80}, // stale phone PTY
		{Kind: SourceKindTTY, TTY: "/dev/pts/7", Width: 120},
	}
	got := Aggregate(in)
	want := []Source{
		{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 200, Session: "main"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Aggregate tmux-authoritative mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestAggregate_NoTmuxFallsBackToUtmp(t *testing.T) {
	// When tmux returns nothing (not running), utmp entries are the
	// only source and pass through as-is.
	in := []Source{
		{Kind: SourceKindTTY, TTY: "/dev/pts/5", Width: 80},
		{Kind: SourceKindTTY, TTY: "/dev/pts/7", Width: 120},
	}
	got := Aggregate(in)
	if len(got) != 2 {
		t.Errorf("Aggregate: want both utmp entries when no tmux, got %+v", got)
	}
}

func TestAggregate_TwoTmuxKept(t *testing.T) {
	// Production path returns one tmux client, but the type allows
	// multiple — Aggregate keeps them all (still tmux-authoritative).
	in := []Source{
		{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 200, Session: "main"},
		{Kind: SourceKindTmux, TTY: "/dev/pts/5", Width: 80, Session: "mobile"},
	}
	got := Aggregate(in)
	if len(got) != 2 {
		t.Errorf("Aggregate: want 2 tmux kept, got %+v", got)
	}
}

func TestAggregate_TmuxFirstUtmpDropped(t *testing.T) {
	// Input mixes tmux and utmp; output is tmux-only.
	in := []Source{
		{Kind: SourceKindTTY, TTY: "/dev/pts/5", Width: 80},
		{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 200, Session: "main"},
	}
	got := Aggregate(in)
	if len(got) != 1 || got[0].Kind != SourceKindTmux {
		t.Errorf("Aggregate: tmux present should drop utmp, got %+v", got)
	}
}

func TestMinWidth_Empty(t *testing.T) {
	w, ok := MinWidth(nil)
	if ok || w != 0 {
		t.Errorf("MinWidth(nil): want (0, false), got (%d, %v)", w, ok)
	}
}

func TestMinWidth_PicksSmallest(t *testing.T) {
	in := []Source{
		{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 200, Session: "main"},
		{Kind: SourceKindTTY, TTY: "/dev/pts/5", Width: 80},
		{Kind: SourceKindTmux, TTY: "/dev/pts/7", Width: 120, Session: "other"},
	}
	w, ok := MinWidth(in)
	if !ok || w != 80 {
		t.Errorf("MinWidth: want (80, true), got (%d, %v)", w, ok)
	}
}

func TestMinWidth_IgnoresNonPositive(t *testing.T) {
	// Defensive: even if a 0 or negative slips through upstream filters,
	// MinWidth must not return it.
	in := []Source{
		{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 0, Session: "main"},
		{Kind: SourceKindTTY, TTY: "/dev/pts/5", Width: -5},
		{Kind: SourceKindTmux, TTY: "/dev/pts/7", Width: 200, Session: "other"},
	}
	w, ok := MinWidth(in)
	if !ok || w != 200 {
		t.Errorf("MinWidth: want (200, true) ignoring 0/-5, got (%d, %v)", w, ok)
	}
}

func TestMinWidth_AllNonPositive(t *testing.T) {
	in := []Source{
		{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 0, Session: "main"},
		{Kind: SourceKindTTY, TTY: "/dev/pts/5", Width: -1},
	}
	w, ok := MinWidth(in)
	if ok || w != 0 {
		t.Errorf("MinWidth: want (0, false) for all non-positive, got (%d, %v)", w, ok)
	}
}

func TestAggregate_DoesNotMutateInput(t *testing.T) {
	in := []Source{
		{Kind: SourceKindTTY, TTY: "/dev/pts/5", Width: 80},
		{Kind: SourceKindTmux, TTY: "/dev/pts/3", Width: 200, Session: "main"},
		{Kind: SourceKindTTY, TTY: "/dev/pts/3", Width: 200}, // would be deduped
	}
	snapshot := append([]Source(nil), in...)

	_ = Aggregate(in)

	if !reflect.DeepEqual(in, snapshot) {
		t.Errorf("Aggregate mutated input:\n got %+v\nwant %+v", in, snapshot)
	}
}
