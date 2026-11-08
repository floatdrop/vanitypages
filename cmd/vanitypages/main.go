// Command vanitypages generates a static site of Go vanity import pages from a
// JSON config file.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/floatdrop/vanitypages"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("vanitypages", flag.ContinueOnError)
	flags.SetOutput(stderr)
	config := flags.String("config", "vanitypages.json", "path to the config file")
	out := flags.String("out", "public", "directory to write the site to")
	schema := flags.Bool("schema", false, "print the config JSON Schema and exit")
	version := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	switch {
	case *schema:
		stdout.Write(vanitypages.Schema())
		return 0
	case *version:
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	cfg, err := vanitypages.Load(*config)
	if err == nil {
		err = cfg.Generate(*out)
	}
	if err != nil {
		fmt.Fprintf(stderr, "vanitypages: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "generated %s with %d modules in %s\n", cfg.Domain, len(cfg.Modules), *out)
	return 0
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(unknown)"
}
