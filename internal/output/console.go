package output

import (
	"fmt"
	"io"
	"time"

	"github.com/spedious/spedious/internal/check"
	"github.com/spedious/spedious/internal/metrics"
)

// ConsoleWriter prints formatted live progress and end-of-test summary.
type ConsoleWriter struct {
	Out io.Writer
}

// NewConsoleWriter creates a ConsoleWriter.
func NewConsoleWriter(out io.Writer) *ConsoleWriter {
	return &ConsoleWriter{Out: out}
}

// PrintBanner prints the Spedious startup banner.
func (c *ConsoleWriter) PrintBanner(scenarioFile string, vus int, duration time.Duration) {
	banner := `
   _____                 _ _                 
  / ____|               | (_)                
 | (___  _ __   ___   __| |_  ___  _   _ ___ 
  \___ \| '_ \ / _ \ / _` + "`" + ` | |/ _ \| | | / __|
  ____) | |_) |  __/| (_| | | (_) | |_| \__ \
 |_____/| .__/ \___| \__,_|_|\___/ \__,_|___/
        | |                                  
        |_|                                  
`
	fmt.Fprintln(c.Out, banner)
	fmt.Fprintf(c.Out, "  Scenario: %s\n", scenarioFile)
	if duration > 0 {
		fmt.Fprintf(c.Out, "  Config  : %d VUs | Duration: %s\n\n", vus, duration)
	} else {
		fmt.Fprintf(c.Out, "  Config  : %d VUs\n\n", vus)
	}
}

// PrintProgressLine updates the live running metrics line.
func (c *ConsoleWriter) PrintProgressLine(s metrics.Snapshot) {
	elapsedSec := s.Duration.Seconds()
	rps := 0.0
	if elapsedSec > 0 {
		rps = float64(s.TotalRequests) / elapsedSec
	}
	errPct := 0.0
	if s.TotalRequests > 0 {
		errPct = (float64(s.TotalErrors) / float64(s.TotalRequests)) * 100.0
	}

	fmt.Fprintf(c.Out, "\r  [running] elapsed: %-6s | VUs: %-4d | reqs: %-6d (%.1f/s) | errs: %-4d (%.1f%%) | p95: %.1fms",
		formatDuration(s.Duration),
		s.ActiveVUs,
		s.TotalRequests,
		rps,
		s.TotalErrors,
		errPct,
		s.P95,
	)
}

// PrintSummary prints the full post-test report and threshold checks.
func (c *ConsoleWriter) PrintSummary(s *metrics.Snapshot, thresholdsPassed bool, thresholdResults []check.Result) {
	fmt.Fprintln(c.Out)
	fmt.Fprintln(c.Out, "  ==================== TEST RESULTS SUMMARY ====================")

	elapsedSec := s.Duration.Seconds()
	if elapsedSec <= 0 {
		elapsedSec = 0.001
	}

	rps := float64(s.TotalRequests) / elapsedSec
	iterRate := float64(s.TotalIterations) / elapsedSec

	fmt.Fprintf(c.Out, "  %-22s: %s\n", "Total Duration", formatDuration(s.Duration))
	fmt.Fprintf(c.Out, "  %-22s: %d (%.1f/s)\n", "Iterations", s.TotalIterations, iterRate)
	fmt.Fprintf(c.Out, "  %-22s: %d (%.1f/s)\n", "Total Requests", s.TotalRequests, rps)
	fmt.Fprintf(c.Out, "  %-22s: %d\n", "Errors", s.TotalErrors)
	fmt.Fprintf(c.Out, "  %-22s: %s\n", "Data Sent", formatBytes(s.BytesSent))
	fmt.Fprintf(c.Out, "  %-22s: %s\n", "Data Received", formatBytes(s.BytesReceived))
	if s.DroppedIterations > 0 {
		fmt.Fprintf(c.Out, "  %-22s: %d\n", "Dropped Iterations", s.DroppedIterations)
	}

	fmt.Fprintln(c.Out, "\n  --- Latency Metrics ---")
	fmt.Fprintf(c.Out, "  http_req_duration.....: min=%.2fms med=%.2fms avg=%.2fms max=%.2fms\n",
		s.Min, s.P50, s.Mean, s.Max)
	fmt.Fprintf(c.Out, "  percentiles...........: p90=%.2fms p95=%.2fms p99=%.2fms p99.9=%.2fms\n",
		s.P90, s.P95, s.P99, s.P999)

	if s.WaitingDuration.Count > 0 || s.ConnectingDuration.Count > 0 {
		fmt.Fprintln(c.Out, "\n  --- Network Timing Breakdown (avg / p95) ---")
		if s.BlockedDuration.Count > 0 {
			fmt.Fprintf(c.Out, "  * blocked.............: avg=%.2fms p95=%.2fms\n", s.BlockedDuration.Mean, s.BlockedDuration.P95)
		}
		if s.ConnectingDuration.Count > 0 {
			fmt.Fprintf(c.Out, "  * connecting..........: avg=%.2fms p95=%.2fms\n", s.ConnectingDuration.Mean, s.ConnectingDuration.P95)
		}
		if s.TLSDuration.Count > 0 {
			fmt.Fprintf(c.Out, "  * tls_handshaking.....: avg=%.2fms p95=%.2fms\n", s.TLSDuration.Mean, s.TLSDuration.P95)
		}
		if s.WaitingDuration.Count > 0 {
			fmt.Fprintf(c.Out, "  * waiting (TTFB)......: avg=%.2fms p95=%.2fms\n", s.WaitingDuration.Mean, s.WaitingDuration.P95)
		}
		if s.ReceivingDuration.Count > 0 {
			fmt.Fprintf(c.Out, "  * receiving...........: avg=%.2fms p95=%.2fms\n", s.ReceivingDuration.Mean, s.ReceivingDuration.P95)
		}
	}

	if s.IterationDuration.Count > 0 {
		fmt.Fprintf(c.Out, "  iteration_duration....: avg=%.2fms p95=%.2fms min=%.2fms max=%.2fms\n",
			s.IterationDuration.Mean, s.IterationDuration.P95, s.IterationDuration.Min, s.IterationDuration.Max)
	}

	if len(s.StepStats) > 0 {
		fmt.Fprintln(c.Out, "\n  --- Step Breakdown ---")
		for name, stat := range s.StepStats {
			fmt.Fprintf(c.Out, "  * %-20s: reqs=%-5d errs=%-3d avg=%.1fms p95=%.1fms min=%.1fms max=%.1fms\n",
				name, stat.Requests, stat.Errors, stat.Mean, stat.P95, stat.Min, stat.Max)
		}
	}

	if len(thresholdResults) > 0 {
		fmt.Fprintln(c.Out, "\n  --- Thresholds ---")
		for _, tr := range thresholdResults {
			icon := "✓"
			status := "PASS"
			if !tr.Passed {
				icon = "✗"
				status = "FAIL"
			}
			msg := ""
			if tr.Message != "" {
				msg = fmt.Sprintf(" (%s)", tr.Message)
			}
			fmt.Fprintf(c.Out, "  %s [%s] %s%s\n", icon, status, tr.Rule, msg)
		}
	}

	fmt.Fprintln(c.Out, "  ==============================================================")
	if thresholdsPassed {
		fmt.Fprintln(c.Out, "  STATUS: PASSED")
	} else {
		fmt.Fprintln(c.Out, "  STATUS: FAILED (Thresholds breached)")
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Millisecond)
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
