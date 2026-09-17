package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: getsloth <command> [args...]")
		os.Exit(2)
	}
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout))
}
