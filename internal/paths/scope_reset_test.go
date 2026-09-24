package paths

import (
	"database/sql"
	"os"
	"testing"
)

func TestResetStaleStoreDeletesUnscopedDatabase(t *testing.T) {
	root := t.TempDir()
	layout := Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", layout.Database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE projects (name TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO projects(name) VALUES('old')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ResetStaleStore(layout); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.Database); !os.IsNotExist(err) {
		t.Fatal("database without scope columns should be removed")
	}
}
