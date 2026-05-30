package bootstrap

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"snzstudio/internal/db"
)

// insertProject adds one project row so a copy can be verified by row count.
func insertProject(t *testing.T, d *sql.DB, id string) {
	t.Helper()
	_, err := d.Exec(
		`INSERT INTO projects (id, title, description, system_prompt, sort_order, created_at, updated_at)
		 VALUES (?, ?, '', '', 0, ?, ?)`,
		id, "Project "+id, "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z",
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
}

func projectCount(t *testing.T, dbPath string) int {
	t.Helper()
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db %s: %v", dbPath, err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow("SELECT count(*) FROM projects").Scan(&n); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	return n
}

// TestSeedDataDirMigrates is the core invariant: a populated source seeds an
// empty destination, the database copy is openable and complete (including rows
// that may still live in the source's -wal — the source connection is kept open
// during the copy so the just-inserted rows are not yet checkpointed), and the
// uploads tree and app-config.json come across verbatim.
func TestSeedDataDirMigrates(t *testing.T) {
	srcDir := t.TempDir()
	destDir := filepath.Join(t.TempDir(), "snz-studio") // not created yet

	src, err := db.Open(filepath.Join(srcDir, "app.sqlite"))
	if err != nil {
		t.Fatalf("open source db: %v", err)
	}
	insertProject(t, src, "1")
	insertProject(t, src, "2")
	// Keep the source connection open: the committed rows may still be in the
	// -wal (no checkpoint yet), so a correct copy must read WAL content.
	defer src.Close()

	if err := os.MkdirAll(filepath.Join(srcDir, "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "uploads", "a.jpg"), []byte("img-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := []byte("{\n  \"llmModel\": \"gpt-oss-20b\"\n}\n")
	if err := os.WriteFile(filepath.Join(srcDir, "app-config.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}

	migrated, err := seedDataDir(srcDir, destDir)
	if err != nil {
		t.Fatalf("seedDataDir: %v", err)
	}
	if !migrated {
		t.Fatal("expected migrated=true")
	}

	if n := projectCount(t, filepath.Join(destDir, "app.sqlite")); n != 2 {
		t.Fatalf("dest projects = %d, want 2 (WAL content lost?)", n)
	}
	if got, _ := os.ReadFile(filepath.Join(destDir, "uploads", "a.jpg")); string(got) != "img-bytes" {
		t.Fatalf("upload not copied verbatim: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(destDir, "app-config.json")); string(got) != string(cfg) {
		t.Fatalf("app-config.json not copied verbatim: %q", got)
	}
}

// TestSeedDataDirSkipsWhenDestPopulated proves the guard that prevents clobbering
// live data: if the destination already has an app.sqlite, nothing is copied.
func TestSeedDataDirSkipsWhenDestPopulated(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	src, err := db.Open(filepath.Join(srcDir, "app.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	insertProject(t, src, "1")
	src.Close()

	sentinel := []byte("existing-database")
	if err := os.WriteFile(filepath.Join(destDir, "app.sqlite"), sentinel, 0o644); err != nil {
		t.Fatal(err)
	}

	migrated, err := seedDataDir(srcDir, destDir)
	if err != nil {
		t.Fatalf("seedDataDir: %v", err)
	}
	if migrated {
		t.Fatal("expected migrated=false when destination already populated")
	}
	if got, _ := os.ReadFile(filepath.Join(destDir, "app.sqlite")); string(got) != string(sentinel) {
		t.Fatalf("destination database was overwritten: %q", got)
	}
}

// TestSeedDataDirSkipsWhenNoSource proves a fresh install (no old data) is a
// clean no-op rather than an error.
func TestSeedDataDirSkipsWhenNoSource(t *testing.T) {
	srcDir := t.TempDir() // empty, no app.sqlite
	destDir := t.TempDir()

	migrated, err := seedDataDir(srcDir, destDir)
	if err != nil {
		t.Fatalf("seedDataDir: %v", err)
	}
	if migrated {
		t.Fatal("expected migrated=false when source has no database")
	}
	if fileExists(filepath.Join(destDir, "app.sqlite")) {
		t.Fatal("destination database should not have been created")
	}
}

// TestMaybeSeedDataDirHonorsEnv proves the env wiring: with SNZ_MIGRATE_FROM set,
// the destination is seeded; with it unset, the call is a no-op.
func TestMaybeSeedDataDirHonorsEnv(t *testing.T) {
	srcDir := t.TempDir()
	src, err := db.Open(filepath.Join(srcDir, "app.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	insertProject(t, src, "1")
	defer src.Close()

	destDir := filepath.Join(t.TempDir(), "snz-studio")
	paths := Paths{
		DataDir:       destDir,
		SQLitePath:    filepath.Join(destDir, "app.sqlite"),
		UploadDir:     filepath.Join(destDir, "uploads"),
		AppConfigPath: filepath.Join(destDir, "app-config.json"),
	}

	// Unset: no-op, no error, nothing created.
	t.Setenv(migrationSourceEnv, "")
	if err := MaybeSeedDataDir(paths); err != nil {
		t.Fatalf("MaybeSeedDataDir (unset): %v", err)
	}
	if fileExists(filepath.Join(destDir, "app.sqlite")) {
		t.Fatal("nothing should be migrated when env is unset")
	}

	// Set: migration runs.
	t.Setenv(migrationSourceEnv, srcDir)
	if err := MaybeSeedDataDir(paths); err != nil {
		t.Fatalf("MaybeSeedDataDir (set): %v", err)
	}
	if n := projectCount(t, filepath.Join(destDir, "app.sqlite")); n != 1 {
		t.Fatalf("dest projects = %d, want 1", n)
	}
}
