package main

import (
	"os"

	"github.com/STR-Consulting/bean-me-up/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
