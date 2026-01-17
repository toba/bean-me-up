package main

import (
	"os"

	"github.com/pacer/bean-me-up/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
