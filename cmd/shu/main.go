package main

import (
	"os"

	"github.com/navisoft0/shuhari/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
