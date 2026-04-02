package cache

import (
	"database/sql"
	"time"
)

const DefaultTTLHours = 24

// IsStale returns true if lastFetched is null or older than ttlHours.
func IsStale(lastFetched sql.NullTime, ttlHours int) bool {
	if !lastFetched.Valid {
		return true
	}
	return time.Since(lastFetched.Time) > time.Duration(ttlHours)*time.Hour
}
