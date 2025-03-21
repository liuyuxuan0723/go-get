package main

import (
	"os"

	"github.com/liuyuxuan0723/go-get/cmd"
)

func main() {
	root := cmd.Root()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
