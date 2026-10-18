package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spedious/spedious/internal/engine"
	"github.com/spedious/spedious/internal/metrics"
	"github.com/spedious/spedious/internal/output"
	"github.com/spedious/spedious/internal/scenario"
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

	validateCmd := &cobra.Command{
		Use:   "validate [scenario-file]",
		Short: "Validate scenario syntax and configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			f, err := scenario.LoadFile(filePath)
			if err != nil {
				return fmt.Errorf("validation error in %s: %w", filePath, err)
			}
			fmt.Printf("✓ Scenario %q (v%d) is valid\n", f.Name, f.Version)
			fmt.Printf("  Scenarios defined: %d\n", len(f.Scenarios))
			for name, sc := range f.Scenarios {
				fmt.Printf("  * %s (executor: %s, steps: %d)\n", name, sc.Load.Executor, len(sc.Steps))
			}
			if len(f.Thresholds) > 0 {
				fmt.Printf("  Thresholds defined: %d\n", len(f.Thresholds))
			}
			return nil
		},
	}

	var (
		flagVUs           int
		flagDuration      time.Duration
		flagQuiet         bool
		flagSummaryExport string
		flagOut           string
		flagAbortOnFail   bool
	)

	runCmd := &cobra.Command{
		Use:   "run [scenario-file]",
		Short: "Run a load test scenario",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			f, err := scenario.LoadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to load scenario %s: %w", filePath, err)
			}

			// Apply CLI flag overrides to scenarios
			totalVUs := 0
			var maxDur time.Duration
			for name, sc := range f.Scenarios {
				if flagVUs > 0 {
					sc.Load.VUs = flagVUs
				}
				if flagDuration > 0 {
					sc.Load.Duration = scenario.Duration(flagDuration)
				}
				f.Scenarios[name] = sc
				totalVUs += sc.Load.VUs
				if sc.Load.Duration.Duration() > maxDur {
					maxDur = sc.Load.Duration.Duration()
				}
			}

			cw := output.NewConsoleWriter(os.Stdout)
			if !flagQuiet {
				cw.PrintBanner(filePath, totalVUs, maxDur)
			}

			// Handle graceful shutdown
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			eng := engine.NewEngine()
			eng.SetAbortOnFail(flagAbortOnFail)

			if !flagQuiet {
				eng.SetProgressCallback(func(snap metrics.Snapshot) {
					cw.PrintProgressLine(snap)
				})
			}

			snap, err := eng.Run(ctx, f)
			if err != nil && err != context.Canceled {
				return fmt.Errorf("execution error: %w", err)
			}

			thresholdsPassed, thresholdResults := engine.EvaluateThresholds(f.Thresholds, snap)
			cw.PrintSummary(snap, thresholdsPassed, thresholdResults)

			// Handle exports
			if flagSummaryExport != "" {
				if err := output.WriteJSONSummary(flagSummaryExport, snap, thresholdsPassed, thresholdResults); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing JSON summary: %v\n", err)
				} else {
					fmt.Printf("  Summary report saved to %s\n", flagSummaryExport)
				}
			}

			if flagOut != "" {
				// Parse --out format, e.g. "csv=path/to/file.csv"
				parts := strings.SplitN(flagOut, "=", 2)
				if len(parts) == 2 && strings.ToLower(parts[0]) == "csv" {
					csvPath := parts[1]
					if err := output.WriteCSVTimeSeries(csvPath, snap.TimeSeries); err != nil {
						fmt.Fprintf(os.Stderr, "Error writing CSV output: %v\n", err)
					} else {
						fmt.Printf("  CSV time-series saved to %s\n", csvPath)
					}
				}
			}

			if !thresholdsPassed {
				os.Exit(1)
			}
			return nil
		},
	}

	runCmd.Flags().IntVarP(&flagVUs, "vus", "u", 0, "Override virtual users count")
	runCmd.Flags().DurationVarP(&flagDuration, "duration", "d", 0, "Override scenario duration (e.g. 10s, 1m)")
	runCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false, "Disable live progress output")
	runCmd.Flags().StringVar(&flagSummaryExport, "summary-export", "", "Path to export JSON summary report")
	runCmd.Flags().StringVar(&flagOut, "out", "", "Export time-series metrics (e.g. --out csv=results.csv)")
	runCmd.Flags().BoolVar(&flagAbortOnFail, "abort-on-fail", false, "Abort run early if any threshold fails")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(runCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
