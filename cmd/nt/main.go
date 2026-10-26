// Command nt keeps plain markdown notes in a git repository: daily journals, topic notes and
// agent reports, read in the terminal and synced by git.
package main

import (
	"os"

	"github.com/w4jnl/nt/internal/cli"
)

func main() { os.Exit(cli.Main(os.Args[1:])) }
