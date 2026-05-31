package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
)

// Command is a single wa-dd subcommand. Each runner uses its own
// flag.FlagSet inside Run so the registry stays free of flag-shape
// concerns.
//
// Run returns the process exit code. 0 = success.
type Command struct {
	Name     string
	Synopsis string
	Run      func(args []string) int
}

var commandRegistry = map[string]Command{}

// register adds c to the registry. Duplicate names panic — they
// indicate a copy/paste bug at init time.
func register(c Command) {
	if c.Name == "" {
		panic("register: empty Command.Name")
	}
	if _, dup := commandRegistry[c.Name]; dup {
		panic(fmt.Sprintf("register: duplicate Command %q", c.Name))
	}
	commandRegistry[c.Name] = c
}

// commandsSorted returns every registered command ordered by Name.
func commandsSorted() []Command {
	out := make([]Command, 0, len(commandRegistry))
	for _, c := range commandRegistry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// printHelp writes the top-level usage banner plus the registered
// command list to stderr.
func printHelp(w *os.File) {
	fmt.Fprintln(w, "wa-dd — Washington Digital Democracy operator CLI")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE:")
	fmt.Fprintln(w, "  wa-dd <subcommand> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SUBCOMMANDS:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, c := range commandsSorted() {
		fmt.Fprintf(tw, "  %s\t%s\n", c.Name, c.Synopsis)
	}
	tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'wa-dd <subcommand> -h' for subcommand flags.")
}
