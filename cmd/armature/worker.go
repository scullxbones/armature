package main

import "github.com/spf13/cobra"

func newWorkerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Worker runtime commands",
	}
	cmd.AddCommand(newWorkerRunCmd())
	return cmd
}
