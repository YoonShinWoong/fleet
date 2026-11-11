package main

import (
	"fmt"
	"os"

	"github.com/YoonShinWoong/keepty/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "keepty:", err)
		os.Exit(1)
	}
}
