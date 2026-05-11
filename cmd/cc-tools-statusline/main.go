// Package main implements the cc-tools-statusline CLI application.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Veraticus/cc-tools/internal/aliases"
	"github.com/Veraticus/cc-tools/internal/output"
	"github.com/Veraticus/cc-tools/internal/statusline"
)

func main() {
	out := output.NewTerminal(os.Stdout, os.Stderr)

	// Read stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		// Fallback prompt output to stdout
		out.Raw(" > ")
		os.Exit(0)
	}

	// Recreate stdin reader
	reader := bytes.NewReader(input)

	result, err := runStatuslineWithInput(reader)
	if err != nil {
		// Fallback prompt output to stdout
		out.Raw(" > ")
		os.Exit(0)
	}
	// Output statusline result to stdout
	out.Raw(result)
}

func runStatuslineWithInput(reader io.Reader) (string, error) {
	resolver, resolverErr := aliases.NewResolver(aliases.DefaultPath())
	if resolverErr != nil {
		fmt.Fprintf(os.Stderr, "cc-tools-statusline: alias file parse error: %v\n", resolverErr)
		// Continue with an empty resolver — better to render with raw labels
		// than to crash the prompt.
		resolver, _ = aliases.NewResolver("")
	}

	deps := &statusline.Dependencies{
		FileReader:    &statusline.DefaultFileReader{},
		CommandRunner: &statusline.DefaultCommandRunner{},
		EnvReader:     &statusline.DefaultEnvReader{},
		TerminalWidth: &statusline.DefaultTerminalWidth{},
		Resolver:      resolver,
		CacheDir:      getCacheDir(),
		CacheDuration: getCacheDuration(),
	}

	sl := statusline.CreateStatusline(deps)

	result, err := sl.Generate(reader)
	if err != nil {
		return "", fmt.Errorf("generating statusline: %w", err)
	}

	return result, nil
}

func getCacheDir() string {
	if dir := os.Getenv("CLAUDE_STATUSLINE_CACHE_DIR"); dir != "" {
		return dir
	}
	return "/dev/shm"
}

func getCacheDuration() time.Duration {
	if os.Getenv("DEBUG_CONTEXT") == "1" {
		return 0
	}
	if seconds := os.Getenv("CLAUDE_STATUSLINE_CACHE_SECONDS"); seconds != "" {
		if duration, err := time.ParseDuration(seconds + "s"); err == nil {
			return duration
		}
	}
	const defaultCacheSeconds = 20
	return defaultCacheSeconds * time.Second
}
