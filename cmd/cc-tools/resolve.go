package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"

	"github.com/Veraticus/cc-tools/internal/aliases"
)

func runResolveCommand() {
	flags := flag.NewFlagSet("resolve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	kindStr := flags.String("type", "", "alias kind: host|aws|k8s|gcloud")
	raw := flags.String("raw", "", "raw value to resolve")
	if err := flags.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}

	kind, err := parseResolveKind(*kindStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cc-tools resolve: %v\n", err)
		os.Exit(2)
	}

	path := aliases.DefaultPath()
	if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "cc-tools resolve: %s missing, using built-in patterns\n", path)
	}

	r, err := aliases.NewResolver(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cc-tools resolve: %v\n", err)
		os.Exit(1)
	}

	label, env := r.Resolve(kind, *raw)
	fmt.Printf("%s\t%s\n", label, env)
}

func parseResolveKind(s string) (aliases.Kind, error) {
	switch s {
	case "host":
		return aliases.KindHost, nil
	case "aws":
		return aliases.KindAWS, nil
	case "k8s":
		return aliases.KindK8s, nil
	case "gcloud":
		return aliases.KindGcloud, nil
	default:
		return 0, fmt.Errorf("unknown --type=%q (want host|aws|k8s|gcloud)", s)
	}
}
