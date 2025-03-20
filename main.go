package main

import (
	"fmt"
	"os"

	"github.com/liuyuxuan0723/go-get/cmd"
	"github.com/liuyuxuan0723/go-get/pkg/mod"
)

func main() {
	root := cmd.NewCommand(mod.NewManager())
	if err := root.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
