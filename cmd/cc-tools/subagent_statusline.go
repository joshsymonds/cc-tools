package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/Veraticus/cc-tools/internal/statusline"
	"github.com/Veraticus/cc-tools/internal/subagentstatusline"
)

const subagentDefaultContextWindow = 1_000_000

func runSubagentStatuslineCommand() {
	code := subagentStatuslineMain()
	if code != 0 {
		os.Exit(code)
	}
}

// subagentStatuslineMain is a separate function so its returns happen
// before the outer os.Exit (gocritic exitAfterDefer pattern, matching
// width_daemon.go).
func subagentStatuslineMain() int {
	contextWindow := subagentDefaultContextWindow
	if raw := os.Getenv("CC_TOOLS_SUBAGENT_CONTEXT_WINDOW"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			contextWindow = n
		}
	}
	env := &statusline.DefaultEnvReader{}
	if err := subagentstatusline.Render(os.Stdin, os.Stdout, contextWindow, env); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "subagent-statusline: %v\n", err)
		return 1
	}
	return 0
}
