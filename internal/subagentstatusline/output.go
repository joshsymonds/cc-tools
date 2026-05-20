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
//
// Each decoration is written as marshaled-bytes + '\n' in a SINGLE
// write call (one append to a per-call buffer, then one Write). This
// collapses 2N syscalls to N when w is unbuffered — and to 1 syscall
// total when callers wrap w in bufio.Writer.
func WriteDecorations(w io.Writer, decorations []Decoration) error {
	for _, d := range decorations {
		encoded, marshalErr := json.Marshal(d)
		if marshalErr != nil {
			return fmt.Errorf("marshal decoration %q: %w", d.ID, marshalErr)
		}
		// Append the newline to the marshaled bytes so the syscall
		// covers both in one shot. encoded is already a fresh slice
		// from json.Marshal, so appending in place is safe.
		encoded = append(encoded, '\n')
		if _, writeErr := w.Write(encoded); writeErr != nil {
			return fmt.Errorf("write decoration %q: %w", d.ID, writeErr)
		}
	}
	return nil
}
