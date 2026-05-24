package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Stdout, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out io.Writer, _ []string) error {
	_, err := fmt.Fprintln(out, "harbormaster placeholder — M1 will replace this")
	return err
}
