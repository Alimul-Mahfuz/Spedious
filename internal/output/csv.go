package output

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spedious/spedious/internal/metrics"
)

// WriteCSVTimeSeries exports time-series metrics samples to a CSV file.
func WriteCSVTimeSeries(filePath string, samples []metrics.TimeSeriesSample) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file %q: %w", filePath, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Header row
	header := []string{
		"timestamp",
		"elapsed_sec",
		"active_vus",
		"rps",
		"errors",
		"error_rate",
		"p95_duration_ms",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, s := range samples {
		row := []string{
			s.Timestamp.Format(time.RFC3339),
			strconv.FormatFloat(s.ElapsedSec, 'f', 2, 64),
			strconv.FormatInt(s.ActiveVUs, 10),
			strconv.FormatFloat(s.RPS, 'f', 1, 64),
			strconv.FormatInt(s.Errors, 10),
			strconv.FormatFloat(s.ErrorRate, 'f', 4, 64),
			strconv.FormatFloat(s.P95Duration, 'f', 2, 64),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
