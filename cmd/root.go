package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	deaddb "github.com/oxGrad/deadgit/internal/db"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
)

var (
	dbPath    string
	outputFmt string
	globalDB  *sql.DB
	globalQ   *dbgen.Queries
)

var rootCmd = &cobra.Command{
	Use:   "deadgit",
	Short: "Scan GitHub and Azure DevOps repositories for inactivity",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		sqlDB, err := deaddb.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		globalDB = sqlDB
		globalQ = dbgen.New(sqlDB)
		return nil
	},
}

// Execute is called from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
	if globalDB != nil {
		globalDB.Close()
	}
}

func init() {
	defaultDB := filepath.Join(mustHomeDir(), ".deadgit", "deadgit.db")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDB, "Path to SQLite database")
	rootCmd.PersistentFlags().StringVar(&outputFmt, "output", "table", "Output format: table | json | csv")

	rootCmd.AddCommand(orgCmd)
	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(scanCmd)
}

func mustHomeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}
