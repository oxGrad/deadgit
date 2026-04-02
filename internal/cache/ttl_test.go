package cache_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/oxGrad/deadgit/internal/cache"
)

func TestIsStale_NullTime(t *testing.T) {
	if !cache.IsStale(sql.NullTime{Valid: false}, 24) {
		t.Error("null time should be stale")
	}
}

func TestIsStale_Fresh(t *testing.T) {
	fresh := sql.NullTime{Valid: true, Time: time.Now().Add(-1 * time.Hour)}
	if cache.IsStale(fresh, 24) {
		t.Error("1h old with 24h TTL should not be stale")
	}
}

func TestIsStale_Expired(t *testing.T) {
	old := sql.NullTime{Valid: true, Time: time.Now().Add(-25 * time.Hour)}
	if !cache.IsStale(old, 24) {
		t.Error("25h old with 24h TTL should be stale")
	}
}
