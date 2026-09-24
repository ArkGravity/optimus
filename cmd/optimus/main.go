// Command optimus is the single Optimus binary. It serves the API and the
// embedded web UI, and carries the operational subcommands used by Compose.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Version is injected at build time with -ldflags "-X main.Version=...".
var Version = "dev"

const defaultConfigPath = "configs/config.yaml"

const usage = `Usage: optimus [command] [flags]

Commands:
  server        Run the HTTP server and embedded web UI (default)
  migrate       Apply database migrations (-dir up|down|status)
  seed          Register permissions and seed the initial administrator
  vault-keygen  Print a new base64-encoded 32-byte vault master key
  version       Print the build version

Run "optimus <command> -h" for command flags.
`

func main() {
	cmd, args := splitCommand(os.Args[1:])
	switch cmd {
	case "server":
		runServer(args)
	case "migrate":
		runMigrate(args)
	case "seed":
		runSeed(args)
	case "vault-keygen":
		if err := generate(os.Stdout); err != nil {
			fail("vault-keygen", err)
		}
	case "version":
		fmt.Println(Version)
	case "help", "-h", "--help":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		printUsage(os.Stderr)
		os.Exit(2)
	}
}

// splitCommand defaults to "server" when no subcommand is given so the bare
// binary (optionally followed by server flags) starts the server.
func splitCommand(args []string) (string, []string) {
	if len(args) == 0 || (strings.HasPrefix(args[0], "-") && args[0] != "-h" && args[0] != "--help") {
		return "server", args
	}
	return args[0], args[1:]
}

func printUsage(w io.Writer) {
	_, _ = io.WriteString(w, usage)
}

func fail(stage string, err error) {
	fmt.Fprintf(os.Stderr, "fatal: %s: %v\n", stage, err)
	os.Exit(1)
}
