package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/liuyuxuan0723/go-get/pkg/mod"
	"github.com/spf13/cobra"
)

func Root() *cobra.Command {
	var (
		verbose bool
		timeout int
	)

	root := &cobra.Command{
		Use:   "go-get [module]",
		Short: "Automatically get the latest compatible version of a Go module",
		Long:  `A tool that determines the latest version of a Go module compatible with your current Go version and runs 'go get' for you.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			m := mod.NewManager(verbose)
			module := args[0]

			var timer *time.Timer
			if timeout > 0 {
				timer = time.AfterFunc(time.Duration(timeout)*time.Second, func() {
					fmt.Fprintf(os.Stderr, "Operation timed out after %d seconds\n", timeout)
					os.Exit(1)
				})
			}

			err := m.GoGet(module)

			if timer != nil {
				timer.Stop()
			}

			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	root.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	root.Flags().IntVarP(&timeout, "timeout", "t", 60, "Set global timeout in seconds (0 means no timeout)")

	return root
}
