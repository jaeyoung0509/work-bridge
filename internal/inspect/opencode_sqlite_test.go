package inspect

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	_ "modernc.org/sqlite"
)

func TestInspectOpenCodeSQLite(t *testing.T) {
	// Create a temporary test database in the expected location
	tmpDir := t.TempDir()
	opencodeDBDir := filepath.Join(tmpDir, ".local", "share", "opencode")
	err := os.MkdirAll(opencodeDBDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create test db dir: %v", err)
	}

	dbPath := filepath.Join(opencodeDBDir, "opencode.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

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
	`

	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO project (id, worktree, root_commit_hash)
		VALUES ('proj_001', '/Users/testuser/project-alpha', 'abc123');
	`)
	if err != nil {
		t.Fatalf("failed to insert project: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO workspace (id, project_id, directory, branch)
		VALUES ('ws_001', 'proj_001', '/Users/testuser/project-alpha', 'main');
	`)
	if err != nil {
		t.Fatalf("failed to insert workspace: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO session (id, project_id, workspace_id, slug, directory, title, version, time_created, time_updated, updated_at)
		VALUES ('ses_001', 'proj_001', 'ws_001', 'alpha-session', '/Users/testuser/project-alpha', 'Alpha Session', '1.3.10', 1712000000000, 1712000100000, '2024-04-01T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO session (id, project_id, workspace_id, slug, directory, title, version, time_created, time_updated, updated_at)
		VALUES ('ses_002', 'proj_001', 'ws_001', 'beta-session', '/Users/testuser/project-alpha', 'Beta Session', '1.3.10', 1712000200000, 1712000300000, '2024-04-01T01:00:00Z');
	`)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO message (id, session_id, text)
		VALUES ('msg_001', 'ses_001', 'Message 1');
	`)
	if err != nil {
		t.Fatalf("failed to insert message: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO message (id, session_id, text)
		VALUES ('msg_002', 'ses_001', 'Message 2');
	`)
	if err != nil {
		t.Fatalf("failed to insert message: %v", err)
	}

	db.Close()

	// Test inspect function
	opts := Options{
		HomeDir:   tmpDir,
		ToolPaths: domain.ToolPaths{},
	}

	sessions, notes, err := inspectOpenCode(opts)
	if err != nil {
		t.Fatalf("inspectOpenCode failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}

	// Verify session data
	found := false
	for _, s := range sessions {
		if s.ID == "ses_001" {
			found = true
			if s.Title != "Alpha Session" {
				t.Errorf("expected title 'Alpha Session', got %q", s.Title)
			}
			if s.ProjectRoot != "/Users/testuser/project-alpha" {
				t.Errorf("expected ProjectRoot '/Users/testuser/project-alpha', got %q", s.ProjectRoot)
			}
			if s.MessageCount != 2 {
				t.Errorf("expected MessageCount 2, got %d", s.MessageCount)
			}
			if s.StoragePath != dbPath {
				t.Errorf("expected StoragePath %q, got %q", dbPath, s.StoragePath)
			}
		}
	}

	if !found {
		t.Error("expected to find ses_001 session")
	}

	if len(notes) == 0 {
		t.Error("expected notes to be populated")
	}

	t.Logf("Found %d sessions", len(sessions))
	t.Logf("Notes: %v", notes)
}

func TestInspectOpenCodeSQLiteTimeUpdatedFallback(t *testing.T) {
	tmpDir := t.TempDir()
	opencodeDBDir := filepath.Join(tmpDir, ".local", "share", "opencode")
	err := os.MkdirAll(opencodeDBDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create test db dir: %v", err)
	}

	dbPath := filepath.Join(opencodeDBDir, "opencode.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	schema := `
		CREATE TABLE IF NOT EXISTS project (
			id TEXT PRIMARY KEY,
			path TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS workspace (
			id TEXT PRIMARY KEY,
			projectId TEXT NOT NULL,
			path TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS session (
			id TEXT PRIMARY KEY,
			projectId TEXT NOT NULL,
			workspaceId TEXT,
			title TEXT NOT NULL,
			time_updated INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS message (
			id TEXT PRIMARY KEY,
			sessionId TEXT NOT NULL,
			text TEXT
		);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO project (id, path) VALUES ('proj_legacy', '/Users/testuser/project-legacy');
		INSERT INTO workspace (id, projectId, path) VALUES ('ws_legacy', 'proj_legacy', '/Users/testuser/project-legacy');
		INSERT INTO session (id, projectId, workspaceId, title, time_updated)
		VALUES ('ses_legacy', 'proj_legacy', 'ws_legacy', 'Legacy Session', 1712000400000);
		INSERT INTO message (id, sessionId, text) VALUES ('msg_legacy', 'ses_legacy', 'Legacy message');
	`); err != nil {
		t.Fatalf("failed to insert legacy test data: %v", err)
	}

	opts := Options{
		HomeDir:   tmpDir,
		ToolPaths: domain.ToolPaths{},
	}

	sessions, _, err := inspectOpenCode(opts)
	if err != nil {
		t.Fatalf("inspectOpenCode failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "ses_legacy" {
		t.Fatalf("expected legacy session id, got %q", sessions[0].ID)
	}
	if sessions[0].ProjectRoot != "/Users/testuser/project-legacy" {
		t.Fatalf("expected legacy project root, got %q", sessions[0].ProjectRoot)
	}
	if sessions[0].UpdatedAt != "1712000400000" {
		t.Fatalf("expected time_updated fallback, got %q", sessions[0].UpdatedAt)
	}
	if sessions[0].MessageCount != 1 {
		t.Fatalf("expected message count 1, got %d", sessions[0].MessageCount)
	}
}
