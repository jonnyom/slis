package main

import (
	"fmt"
	"os"

	"github.com/jonnyom/slis/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "slis:", err)
		os.Exit(1)
	}
}
