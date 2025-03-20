package cmd

import (
	"os"
	"time"

	"github.com/liuyuxuan0723/go-get/pkg/mod"
	"github.com/spf13/cobra"
)

func NewCommand(m *mod.Manager) *cobra.Command {
	var (
		debug   bool
		timeout int
		// batchSize int
	)

	root := &cobra.Command{
		Use:   "go-get [module]",
		Short: "Automatically get the latest compatible version of a Go module",
		Long:  `A tool that determines the latest version of a Go module compatible with your current Go version and runs 'go get' for you.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			module := args[0]

			if timeout > 0 {
				time.AfterFunc(time.Duration(timeout)*time.Second, func() {
					os.Exit(1)
				})
			}

			// 执行go get
			if err := m.GoGet(module); err != nil {
				os.Exit(1)
			}
		},
	}

	root.Flags().BoolVarP(&debug, "debug", "d", false, "启用详细调试日志")
	root.Flags().IntVarP(&timeout, "timeout", "t", 120, "设置全局超时时间(秒)，0表示无超时")

	return root
}
