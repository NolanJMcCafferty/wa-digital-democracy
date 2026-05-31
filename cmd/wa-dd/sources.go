package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"

	// Blank imports register every shipped connector with
	// internal/sources/connector so `wa-dd sources` shows the full set
	// even if a particular subcommand isn't otherwise wired up here.
	_ "github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/bls"
	_ "github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/census"
	_ "github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/committeeschedules"
	_ "github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/epa"
	_ "github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/fema"
	_ "github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/hud"
	_ "github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/kingcounty"
	_ "github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/sao"
	_ "github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/seattleauditor"
)

func init() {
	register(Command{
		Name:     "sources",
		Synopsis: "List registered source connectors (system, base URL, description)",
		Run:      runListSources,
	})
}

func runListSources(args []string) int {
	fs := flag.NewFlagSet("sources", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "wa-dd sources — list registered source connectors")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SYSTEM\tBASE URL\tDESCRIPTION")
	for _, d := range connector.Registered() {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", d.System, d.BaseURL, d.Description)
	}
	tw.Flush()
	return 0
}
