package main

import (
	"os"

	"github.com/liuyuxuan0723/go-get/cmd"
	"github.com/liuyuxuan0723/go-get/pkg/mod"
)

func main() {
	root := cmd.Root(mod.NewManager())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
