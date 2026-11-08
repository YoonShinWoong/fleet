package main

import (
	"fmt"
	"os"

	"github.com/YoonShinWoong/fleet/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fleet:", err)
		os.Exit(1)
	}
}
