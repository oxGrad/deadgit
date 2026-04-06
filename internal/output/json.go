package output

import (
	"encoding/json"
	"os"
	"time"
)

// JSONReport is the top-level envelope written to file.
type JSONReport struct {
	Profile        string    `json:"profile"`
	ProfileVersion int       `json:"profile_version"`
	ScannedAt      time.Time `json:"scanned_at"`
	Repos          []RepoRow `json:"repos"`
}

// WriteJSON writes rows as pretty-printed JSON to path with profile metadata envelope.
func WriteJSON(path string, rows []RepoRow, profileName string, profileVersion int) (err error) {
	report := JSONReport{
		Profile:        profileName,
		ProfileVersion: profileVersion,
		ScannedAt:      time.Now().UTC(),
		Repos:          rows,
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
