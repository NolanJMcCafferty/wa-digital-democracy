// Command wa-dd is the operator-facing CLI for ingestion, matching, and
// rendering. Each subcommand lives in its own cmd_*.go file and registers
// itself with the package-level command registry (commands.go) at init
// time, so adding or removing a subcommand never touches main().
package main

import (
	"fmt"
	"os"
)

// version is overridden via -ldflags="-X main.version=…" at release time.
var version = "0.0.0-dev"

var userAgent = env("SOURCE_USER_AGENT", "wa-dd/0.0.1 (https://github.com/nolan-mccafferty/wa-digital-democracy; nolan-mccafferty)")

func init() {
	register(Command{
		Name:     "version",
		Synopsis: "Print version info",
		Run: func(args []string) int {
			fmt.Println(version)
			return 0
		},
	})
	// Phase-3 stubs: ingest-bill / ingest-csi / match-hearing remain
	// internal pipeline steps. The routine operator path is
	// ingest-session -> ingest-hearings.
	for _, n := range []string{"ingest-bill", "ingest-csi", "match-hearing"} {
		name := n
		register(Command{
			Name:     name,
			Synopsis: "(internal pipeline step; use ingest-session/ingest-hearings)",
			Run: func(args []string) int {
				fmt.Fprintf(os.Stderr, "wa-dd %s: direct per-step CLI not exposed; use ingest-session/ingest-hearings\n", name)
				return 64
			},
		})
	}
}

func main() {
	if len(os.Args) < 2 {
		printHelp(os.Stderr)
		os.Exit(2)
	}
	name, args := os.Args[1], os.Args[2:]
	switch name {
	case "-h", "--help", "help":
		printHelp(os.Stdout)
		return
	}
	cmd, ok := commandRegistry[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "wa-dd: unknown subcommand %q\n\n", name)
		printHelp(os.Stderr)
		os.Exit(2)
	}
	os.Exit(cmd.Run(args))
}
