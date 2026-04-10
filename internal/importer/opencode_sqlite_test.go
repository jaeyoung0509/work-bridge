package importer

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenCodeSQLiteFixture(t *testing.T) {
	// Create a temporary test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "opencode.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()
	defer os.Remove(dbPath)

	// Create schema matching OpenCode tables
	schema := `
		CREATE TABLE IF NOT EXISTS project (
			id TEXT PRIMARY KEY,
			worktree TEXT NOT NULL,
			root_commit_hash TEXT
		);
		CREATE TABLE IF NOT EXISTS workspace (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			directory TEXT NOT NULL,
			branch TEXT,
			FOREIGN KEY (project_id) REFERENCES project(id)
		);
		CREATE TABLE IF NOT EXISTS session (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			workspace_id TEXT,
			slug TEXT NOT NULL,
			directory TEXT NOT NULL,
			title TEXT NOT NULL,
			version TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			updated_at TEXT,
			updatedAt TEXT,
			FOREIGN KEY (project_id) REFERENCES project(id),
			FOREIGN KEY (workspace_id) REFERENCES workspace(id)
		);
		CREATE TABLE IF NOT EXISTS message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			text TEXT,
			content TEXT,
			FOREIGN KEY (session_id) REFERENCES session(id)
		);
		CREATE TABLE IF NOT EXISTS part (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			text TEXT,
			content TEXT,
			FOREIGN KEY (message_id) REFERENCES message(id),
			FOREIGN KEY (session_id) REFERENCES session(id)
		);
	`

	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO project (id, worktree, root_commit_hash)
		VALUES ('proj_test_001', '/Users/testuser/myproject', 'abc123def456');
	`)
	if err != nil {
		t.Fatalf("failed to insert project: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO workspace (id, project_id, directory, branch)
		VALUES ('ws_test_001', 'proj_test_001', '/Users/testuser/myproject', 'main');
	`)
	if err != nil {
		t.Fatalf("failed to insert workspace: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO session (id, project_id, workspace_id, slug, directory, title, version, time_created, time_updated, updated_at, updatedAt)
		VALUES ('ses_test_001', 'proj_test_001', 'ws_test_001', 'test-session', '/Users/testuser/myproject', 'Test Session Title', '1.3.10', 1712000000000, 1712000100000, '2024-04-01T00:00:00Z', '2024-04-01T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO message (id, session_id, text)
		VALUES ('msg_test_001', 'ses_test_001', 'User test message');
	`)
	if err != nil {
		t.Fatalf("failed to insert message: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO part (id, message_id, session_id, text)
		VALUES ('part_test_001', 'msg_test_001', 'ses_test_001', 'Assistant response part 1');
	`)
	if err != nil {
		t.Fatalf("failed to insert part: %v", err)
	}

	// Test the import function
	raw := &RawImportResult{
		SourceTool:      "opencode",
		SourceSessionID: "ses_test_001",
	}

	err = importOpenCodeSQLite(dbPath, "ses_test_001", raw)
	if err != nil {
		t.Fatalf("importOpenCodeSQLite failed: %v", err)
	}

	// Verify results
	if raw.ProjectRoot != "/Users/testuser/myproject" {
		t.Errorf("expected ProjectRoot '/Users/testuser/myproject', got %q", raw.ProjectRoot)
	}

	if raw.Summary == "" {
		t.Error("expected Summary to be populated, got empty string")
	}

	t.Logf("ProjectRoot: %s", raw.ProjectRoot)
	t.Logf("Summary: %s", raw.Summary)
	t.Logf("ResumeHints: %v", raw.ResumeHints)
}

func TestOpenCodeSQLiteMissingSession(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "opencode.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()
	defer os.Remove(dbPath)

	// Create minimal schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS session (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			workspace_id TEXT,
			slug TEXT NOT NULL,
			directory TEXT NOT NULL,
			title TEXT NOT NULL,
			version TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			updated_at TEXT,
			updatedAt TEXT
		);
		CREATE TABLE IF NOT EXISTS message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			text TEXT
		);
		CREATE TABLE IF NOT EXISTS part (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			text TEXT
		);
	`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Insert a different session
	_, err = db.Exec(`
		INSERT INTO session (id, project_id, workspace_id, slug, directory, title, version, time_created, time_updated)
		VALUES ('ses_other', 'proj_other', 'ws_other', 'other', '/other/path', 'Other Session', '1.3.10', 1712000000000, 1712000100000);
	`)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	// Try to import non-existent session - should not panic or error badly
	raw := &RawImportResult{
		SourceTool:      "opencode",
		SourceSessionID: "ses_nonexistent",
	}

	err = importOpenCodeSQLite(dbPath, "ses_nonexistent", raw)
	// Function should handle missing session gracefully (return nil or empty result)
	if err != nil {
		t.Logf("importOpenCodeSQLite returned error (expected behavior): %v", err)
	}
}
