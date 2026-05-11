package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Veraticus/cc-tools/internal/aliases"
)

func runResolveCommand() {
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	kindStr := fs.String("type", "", "alias kind: host|aws|k8s|gcloud")
	raw := fs.String("raw", "", "raw value to resolve")
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}

	kind, err := parseResolveKind(*kindStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cc-tools resolve: %v\n", err)
		os.Exit(2)
	}

	path := aliases.DefaultPath()
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		fmt.Fprintf(os.Stderr, "cc-tools resolve: %s missing, using built-in patterns\n", path)
	}

	r, err := aliases.NewResolver(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cc-tools resolve: %v\n", err)
		os.Exit(1)
	}

	label, env := r.Resolve(kind, *raw)
	fmt.Printf("%s\t%s", label, env)
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
