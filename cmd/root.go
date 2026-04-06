package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	deaddb "github.com/oxGrad/deadgit/internal/db"
	dbgen "github.com/oxGrad/deadgit/internal/db/generated"
)

var (
	dbPath    string
	outputFmt string
	globalDB  *sql.DB
	globalQ   *dbgen.Queries
	globalLog *zap.Logger
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
	globalLog = newLogger()
	defer globalLog.Sync() //nolint:errcheck

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
	if globalDB != nil {
		if err := globalDB.Close(); err != nil {
			globalLog.Error("close database", zap.Error(err))
		}
	}
}

// newLogger returns a human-readable logger. Debug level is enabled when DG_DEBUG=true.
func newLogger() *zap.Logger {
	cfg := zap.NewDevelopmentConfig()
	if os.Getenv("DG_DEBUG") != "true" {
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	l, _ := cfg.Build(zap.WithCaller(false))
	return l
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
