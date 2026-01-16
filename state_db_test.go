package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClearFoundBlocks(t *testing.T) {
	dir := t.TempDir()
	db, err := openStateDB(stateDBPathFromDataDir(dir))
	if err != nil {
		t.Fatalf("openStateDB: %v", err)
	}
	_, err = db.Exec("INSERT INTO found_blocks_log (created_at_unix, json) VALUES (?, ?)", time.Now().Unix(), `{"height":1}`)
	if err != nil {
		_ = db.Close()
		t.Fatalf("insert: %v", err)
	}
	_ = db.Close()

	legacy := filepath.Join(dir, "state", "found_blocks.jsonl")
	if err := os.WriteFile(legacy, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	deleted, err := clearFoundBlocks(dir)
	if err != nil {
		t.Fatalf("clearFoundBlocks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}
	if _, err := os.Stat(legacy); err == nil {
		t.Fatalf("legacy file still exists")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat legacy: %v", err)
	}

	db2, err := openStateDB(stateDBPathFromDataDir(dir))
	if err != nil {
		t.Fatalf("openStateDB: %v", err)
	}
	defer db2.Close()
	var count int
	if err := db2.QueryRow("SELECT COUNT(*) FROM found_blocks_log").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count=%d, want 0", count)
	}
}
