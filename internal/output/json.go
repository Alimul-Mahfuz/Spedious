package output

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spedious/spedious/internal/check"
	"github.com/spedious/spedious/internal/metrics"
)

// JSONReport represents the structured report saved to a JSON file.
type JSONReport struct {
	Metrics    metrics.Snapshot `json:"metrics"`
	Thresholds []check.Result   `json:"thresholds"`
	Passed     bool             `json:"passed"`
}

// WriteJSONSummary exports the complete test metrics and thresholds to a JSON file.
func WriteJSONSummary(filePath string, snap *metrics.Snapshot, passed bool, thresholds []check.Result) error {
	report := JSONReport{
		Metrics:    *snap,
		Thresholds: thresholds,
		Passed:     passed,
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create JSON file %q: %w", filePath, err)
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("failed to encode JSON summary: %w", err)
	}

	return nil
}
