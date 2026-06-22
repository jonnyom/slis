package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "slis:", err)
		os.Exit(1)
	}
}

// run is replaced by cli.Execute in Task 17; stub keeps the binary buildable now.
func run(_ []string) error { return nil }
