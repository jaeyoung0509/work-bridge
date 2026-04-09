package inspect

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	_ "modernc.org/sqlite"
)

func inspectOpenCode(opts Options) ([]Session, []string, error) {
	paths := []string{
		opts.ToolPaths.Dir(domain.ToolOpenCode, opts.HomeDir),
		filepath.Join(opts.HomeDir, ".local", "share", "opencode", "opencode.db"),
		filepath.Join(opts.HomeDir, "Library", "Application Support", "opencode", "opencode.db"),
	}

	var dbPath string
	for _, p := range paths {
		if stat, err := os.Stat(p); err == nil && !stat.IsDir() && strings.HasSuffix(p, ".db") {
			dbPath = p
			break
		}
		candidate := filepath.Join(p, "opencode.db")
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			dbPath = candidate
			break
		}
	}

	if dbPath == "" {
		return nil, []string{"OpenCode SQLite DB not found."}, nil
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open opencode db: %w", err)
	}
	defer db.Close()

	// Query basic session metadata.
	rows, err := db.Query("SELECT id, title, updated_at, project_id, workspace_id FROM session")
	if err != nil {
		rows, err = db.Query("SELECT id, title, updatedAt as updated_at, projectId as project_id, workspaceId as workspace_id FROM session")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to query opencode session table: %w", err)
		}
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var id, title, updatedAt, projectId, workspaceId sql.NullString
		if err := rows.Scan(&id, &title, &updatedAt, &projectId, &workspaceId); err != nil {
			continue
		}
		
		s := Session{
			ID:          id.String,
			Title:       title.String,
			UpdatedAt:   updatedAt.String,
			StoragePath: dbPath,
		}

		if workspaceId.Valid && workspaceId.String != "" {
			var dir string
			if err := db.QueryRow("SELECT directory FROM workspace WHERE id = ?", workspaceId.String).Scan(&dir); err != nil {
				db.QueryRow("SELECT path FROM workspace WHERE id = ?", workspaceId.String).Scan(&dir) // fallback
			}
			s.ProjectRoot = dir
		}
		if s.ProjectRoot == "" && projectId.Valid && projectId.String != "" {
			var dir string
			if err := db.QueryRow("SELECT directory FROM project WHERE id = ?", projectId.String).Scan(&dir); err != nil {
				db.QueryRow("SELECT path FROM project WHERE id = ?", projectId.String).Scan(&dir) // fallback
			}
			s.ProjectRoot = dir
		}
		
		// Attempt to count messages
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM message WHERE session_id = ?", id.String).Scan(&count); err != nil {
			db.QueryRow("SELECT COUNT(*) FROM message WHERE sessionId = ?", id.String).Scan(&count) // fallback
		}
		s.MessageCount = count

		sessions = append(sessions, s)
	}

	sortSessions(sessions)

	return sessions, []string{fmt.Sprintf("Read OpenCode sessions from %s", dbPath)}, nil
}
