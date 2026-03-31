package report

import (
	"encoding/json"
	"fmt"
	"os"
)

// WriteJSON writes reports as pretty-printed JSON to outPath.
func WriteJSON(reports []RepoReport, outPath string) error {
	data, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return fmt.Errorf("write JSON file %s: %w", outPath, err)
	}
	return nil
}
