package subagentstatusline

import (
	"fmt"
	"io"
)

// Render is the package's top-level orchestrator. Reads the
// subagentStatusLine JSON blob from r, calls BuildContent per task,
// and writes one newline-delimited Decoration per task to w.
//
// contextWindow defaults to defaultContextWindow (1_000_000, Opus 1M)
// when <= 0. env supplies AWS_PROFILE / gcloud / k8s.
//
// Returns wrapped errors for input parse failures or output write
// failures. Errors are intended to surface to stderr at the cmd/
// layer; this function never writes to stderr itself.
func Render(r io.Reader, w io.Writer, contextWindow int, env EnvReader) error {
	in, err := Parse(r)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	decorations := make([]Decoration, 0, len(in.Tasks))
	for _, task := range in.Tasks {
		decorations = append(decorations, Decoration{
			ID:      task.ID,
			Content: BuildContent(task, in.Columns, contextWindow, env),
		})
	}
	if writeErr := WriteDecorations(w, decorations); writeErr != nil {
		return fmt.Errorf("render: %w", writeErr)
	}
	return nil
}
