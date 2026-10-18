package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "0.1.0-dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "spedious",
		Aliases: []string{"spd"},
		Short:   "Spedious is an open-source high-performance API load & throttle tester",
		Long: `Spedious (spd) simulates concurrent virtual users running scripted API workflows
with low overhead and accurate latency accounting (avoiding coordinated omission).`,
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the Spedious version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("spedious version %s (commit: %s, built: %s)\n", version, commit, date)
		},
	}

	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
