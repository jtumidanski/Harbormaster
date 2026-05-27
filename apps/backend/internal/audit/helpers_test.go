package audit_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

// newTestDB opens an in-process SQLite database in a temporary directory
// and runs all migrations. The *sql.DB closer is registered with t.Cleanup.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "audit_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// newTestProcessor returns a Processor backed by a fresh in-memory test DB.
func newTestProcessor(t *testing.T) *audit.Processor {
	t.Helper()
	gdb := newTestDB(t)
	return audit.NewProcessor(gdb, 90*24*time.Hour)
}

// loadLatest returns the raw payload_summary_json string for the most-recently
// inserted event with the given action. It fails the test if no such event exists.
func loadLatest(t *testing.T, p *audit.Processor, action string) string {
	t.Helper()
	events, err := audit.List(p.DB(), audit.Filter{Action: action, PageSize: 1})
	require.NoError(t, err)
	require.NotEmpty(t, events, "no events found for action %q", action)

	// Re-read the raw JSON column directly because Event.PayloadSummary is the
	// already-decoded map; we want to assert on the stored string form.
	type row struct {
		PayloadSummaryJSON string `gorm:"column:payload_summary_json"`
	}
	var r row
	require.NoError(t,
		p.DB().
			Table("audit_events").
			Select("payload_summary_json").
			Where("id = ?", events[0].ID).
			Scan(&r).Error,
	)
	return r.PayloadSummaryJSON
}
