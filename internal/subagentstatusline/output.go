package subagentstatusline

import (
	"encoding/json"
	"fmt"
	"io"
)

// Decoration is the per-row output Claude consumes. Schema fixed by
// Claude's Wr5 zod validator: exactly two string fields, no nesting.
type Decoration struct {
	// ID matches a Task.ID from the input. Decorations whose ID
	// doesn't match any active task are silently dropped by Claude.
	ID string `json:"id"`

	// Content is the ANSI-rendered chip chain. May include 24-bit
	// true-color escapes — Claude renders them as-is within the
	// agent panel row.
	Content string `json:"content"`
}

// WriteDecorations emits decorations as newline-delimited JSON. NOT a
// JSON array — Claude reads line-by-line. The first write error
// aborts; remaining decorations are not attempted.
func WriteDecorations(w io.Writer, decorations []Decoration) error {
	for _, d := range decorations {
		encoded, marshalErr := json.Marshal(d)
		if marshalErr != nil {
			return fmt.Errorf("marshal decoration %q: %w", d.ID, marshalErr)
		}
		if _, writeErr := w.Write(encoded); writeErr != nil {
			return fmt.Errorf("write decoration %q: %w", d.ID, writeErr)
		}
		if _, writeErr := w.Write([]byte{'\n'}); writeErr != nil {
			return fmt.Errorf("write newline after %q: %w", d.ID, writeErr)
		}
	}
	return nil
}
