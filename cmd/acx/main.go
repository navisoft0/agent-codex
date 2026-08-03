package main

import (
	"os"

	"github.com/navisoft0/agent-codex/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
