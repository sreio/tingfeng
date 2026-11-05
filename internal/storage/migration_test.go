package storage

import (
	"context"
	"testing"
)

func TestFieldSnapshotMigrationPreservesOldRecords(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`ALTER TABLE records DROP COLUMN fields;
	INSERT INTO records(monitor_id,at,source_at,value,display) VALUES('legacy',1,0,'"90441"','90441');
	PRAGMA user_version=1;`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, err := s.RecordsAfter(context.Background(), "legacy", 0)
	if err != nil || len(rows) != 1 || rows[0].Display != "90441" || len(rows[0].Fields) != 0 {
		t.Fatal(rows, err)
	}
	var version int
	if err = s.DB.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 2 {
		t.Fatal(version, err)
	}
	if err = s.migrate(); err != nil {
		t.Fatal("migration is not idempotent", err)
	}
}
