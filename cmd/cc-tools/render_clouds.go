package main

import (
	"fmt"
	"os"

	"github.com/Veraticus/cc-tools/internal/aliases"
	"github.com/Veraticus/cc-tools/internal/statusline"
)

// runRenderCloudsCommand prints the AWS/gcloud/k8s chip chain as raw
// ANSI for embedding in starship's right_format. Intended to be invoked
// from a starship custom module:
//
//	[custom.cloud_section]
//	when = "true"
//	command = "cc-tools render-clouds"
//	format = "$output"
//
// Output always includes at least the closing right curve in sky (git's
// color), so the right side of the prompt seals correctly even when no
// cloud chips are present.
func runRenderCloudsCommand() {
	resolver, err := aliases.NewResolver(aliases.DefaultPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "cc-tools render-clouds: %v\n", err)
		resolver, _ = aliases.NewResolver("")
	}

	deps := &statusline.Dependencies{
		FileReader:    &statusline.DefaultFileReader{},
		CommandRunner: &statusline.DefaultCommandRunner{},
		EnvReader:     &statusline.DefaultEnvReader{},
		TerminalWidth: &statusline.DefaultTerminalWidth{},
		Resolver:      resolver,
	}

	fmt.Print(statusline.RenderClouds(deps))
}
