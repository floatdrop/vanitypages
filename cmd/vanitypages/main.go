// Command vanitypages generates a static site of Go vanity import pages from a
// JSON config file.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/floatdrop/vanitypages"
)

const detailsTimeout = time.Minute

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("vanitypages", flag.ContinueOnError)
	flags.SetOutput(stderr)
	config := flags.String("config", "vanitypages.json", "path to the config file")
	out := flags.String("out", "public", "directory to write the site to")
	details := flags.Bool("details", false, "add published versions, licenses and synopses from the documentation site")
	schema := flags.Bool("schema", false, "print the config JSON Schema and exit")
	version := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	var err error
	switch {
	case *schema:
		_, err = stdout.Write(vanitypages.Schema())
	case *version:
		_, err = fmt.Fprintln(stdout, buildVersion())
	default:
		var cfg vanitypages.Config
		cfg, err = vanitypages.Load(*config)
		if err == nil && *details {
			err = addDetails(&cfg, stderr)
		}
		if err == nil {
			err = cfg.Generate(*out)
		}
		if err == nil {
			_, err = fmt.Fprintf(stdout, "generated %s with %d modules in %s\n", cfg.Domain, len(cfg.Modules), *out)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "vanitypages: %v\n", err)
		return 1
	}
	return 0
}

// addDetails fills in what the documentation site knows. Pages work without
// it, so only an invalid config is an error.
func addDetails(cfg *vanitypages.Config, stderr io.Writer) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), detailsTimeout)
	defer cancel()
	details, err := cfg.FetchDetails(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "vanitypages: warning: details are incomplete: %v\n", err)
	}
	cfg.Details = details
	return nil
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(unknown)"
}
