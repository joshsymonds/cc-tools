package widthdaemon

// Aggregate folds the per-source detections into the slice the cache
// will be written from.
//
// When tmux is running and has reported at least one client, tmux is
// fully authoritative — utmp's entries are dropped entirely. Rationale:
// tmux's source already picks the most-recently-active client; utmp
// would otherwise resurrect stale PTYs (a phone session left attached
// hours ago whose underlying device still reports its old width) and
// drag the eventual MinWidth back down to that stale value.
//
// When tmux is not running, utmp is the fallback and all of its
// entries are returned as-is.
//
// Pure function: the input slice is not modified.
func Aggregate(sources []Source) []Source {
	if len(sources) == 0 {
		return nil
	}

	tmuxCount := 0
	for _, s := range sources {
		if s.Kind == SourceKindTmux {
			tmuxCount++
		}
	}

	// tmux present → tmux-only. Drop all utmp.
	if tmuxCount > 0 {
		out := make([]Source, 0, tmuxCount)
		for _, s := range sources {
			if s.Kind == SourceKindTmux {
				out = append(out, s)
			}
		}
		return out
	}

	// No tmux → fall back to utmp entries as-is.
	out := make([]Source, 0, len(sources))
	for _, s := range sources {
		if s.Kind != SourceKindTmux {
			out = append(out, s)
		}
	}
	return out
}

// MinWidth returns the smallest positive Width across sources. If no
// source has a positive width, returns (0, false).
//
// In tmux-dominant deployments the input is typically a single source
// (the most-recently-active tmux client) — so this devolves to just
// "that one width." The min behavior matters only for the utmp
// fallback path, where multiple PTYs may be reported and the safer
// choice is the narrowest viewer.
func MinWidth(sources []Source) (int, bool) {
	minWidth := 0
	found := false
	for _, s := range sources {
		if s.Width <= 0 {
			continue
		}
		if !found || s.Width < minWidth {
			minWidth = s.Width
			found = true
		}
	}
	return minWidth, found
}
