package widthdaemon

// Aggregate returns sources reordered so tmux entries come first, with
// any utmp ("tty") entry whose TTY matches a tmux entry dropped. tmux
// is authoritative for any PTY it owns — it knows the actual rendered
// width inside its pane, which can differ from the raw winsize of the
// underlying device.
//
// Pure function: the input slice is not modified.
func Aggregate(sources []Source) []Source {
	if len(sources) == 0 {
		return nil
	}

	tmuxTTYs := make(map[string]struct{}, len(sources))
	tmuxSources := make([]Source, 0, len(sources))
	ttySources := make([]Source, 0, len(sources))

	for _, s := range sources {
		if s.Kind == SourceKindTmux {
			tmuxTTYs[s.TTY] = struct{}{}
			tmuxSources = append(tmuxSources, s)
		}
	}
	for _, s := range sources {
		if s.Kind != SourceKindTmux {
			if _, dup := tmuxTTYs[s.TTY]; dup {
				continue
			}
			ttySources = append(ttySources, s)
		}
	}
	return append(tmuxSources, ttySources...)
}

// MinWidth returns the smallest positive Width across sources. If no
// source has a positive width, returns (0, false). This matches tmux's
// own behavior for multi-client sessions and ensures the cached width
// fits inside the smallest currently-attached viewer.
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
