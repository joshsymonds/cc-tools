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

func TestAggregate_TmuxAndUtmpSameTTYDeduped(t *testing.T) {
	in := []Source{
		{Kind: "tmux", TTY: "/dev/pts/3", Width: 200, Session: "main"},
		{Kind: "tty", TTY: "/dev/pts/3", Width: 200}, // duplicate of tmux
		{Kind: "tty", TTY: "/dev/pts/5", Width: 80},  // distinct
	}
	got := Aggregate(in)
	want := []Source{
		{Kind: "tmux", TTY: "/dev/pts/3", Width: 200, Session: "main"},
		{Kind: "tty", TTY: "/dev/pts/5", Width: 80},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Aggregate dedup mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestAggregate_TwoTmuxKept(t *testing.T) {
	in := []Source{
		{Kind: "tmux", TTY: "/dev/pts/3", Width: 200, Session: "main"},
		{Kind: "tmux", TTY: "/dev/pts/5", Width: 80, Session: "mobile"},
	}
	got := Aggregate(in)
	if len(got) != 2 {
		t.Errorf("Aggregate: want 2 tmux kept, got %+v", got)
	}
}

func TestAggregate_StableOrderTmuxFirst(t *testing.T) {
	// utmp entries come first in input — Aggregate must reorder to tmux-then-utmp.
	in := []Source{
		{Kind: "tty", TTY: "/dev/pts/5", Width: 80},
		{Kind: "tmux", TTY: "/dev/pts/3", Width: 200, Session: "main"},
	}
	got := Aggregate(in)
	if len(got) != 2 || got[0].Kind != "tmux" || got[1].Kind != "tty" {
		t.Errorf("Aggregate: want tmux-then-utmp order, got %+v", got)
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
		{Kind: "tmux", TTY: "/dev/pts/3", Width: 200, Session: "main"},
		{Kind: "tty", TTY: "/dev/pts/5", Width: 80},
		{Kind: "tmux", TTY: "/dev/pts/7", Width: 120, Session: "other"},
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
		{Kind: "tmux", TTY: "/dev/pts/3", Width: 0, Session: "main"},
		{Kind: "tty", TTY: "/dev/pts/5", Width: -5},
		{Kind: "tmux", TTY: "/dev/pts/7", Width: 200, Session: "other"},
	}
	w, ok := MinWidth(in)
	if !ok || w != 200 {
		t.Errorf("MinWidth: want (200, true) ignoring 0/-5, got (%d, %v)", w, ok)
	}
}

func TestMinWidth_AllNonPositive(t *testing.T) {
	in := []Source{
		{Kind: "tmux", TTY: "/dev/pts/3", Width: 0, Session: "main"},
		{Kind: "tty", TTY: "/dev/pts/5", Width: -1},
	}
	w, ok := MinWidth(in)
	if ok || w != 0 {
		t.Errorf("MinWidth: want (0, false) for all non-positive, got (%d, %v)", w, ok)
	}
}
