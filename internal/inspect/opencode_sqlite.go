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
	defer func() {
		_ = db.Close()
	}()

	sessionCols, err := tableColumns(db, "session")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to inspect opencode session schema: %w", err)
	}

	updatedExpr := "NULL"
	switch {
	case sessionCols["updated_at"]:
		updatedExpr = "updated_at"
	case sessionCols["updatedAt"]:
		updatedExpr = "updatedAt"
	case sessionCols["time_updated"]:
		updatedExpr = "CAST(time_updated AS TEXT)"
	}

	projectExpr := "NULL"
	switch {
	case sessionCols["project_id"]:
		projectExpr = "project_id"
	case sessionCols["projectId"]:
		projectExpr = "projectId"
	}

	workspaceExpr := "NULL"
	switch {
	case sessionCols["workspace_id"]:
		workspaceExpr = "workspace_id"
	case sessionCols["workspaceId"]:
		workspaceExpr = "workspaceId"
	}

	// Query basic session metadata.
	query := fmt.Sprintf(
		"SELECT id, title, %s AS updated_at, %s AS project_id, %s AS workspace_id FROM session",
		updatedExpr,
		projectExpr,
		workspaceExpr,
	)
	rows, err := db.Query(query)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query opencode session table: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var sessions []Session
	warnings := []string{}
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
				if err2 := db.QueryRow("SELECT path FROM workspace WHERE id = ?", workspaceId.String).Scan(&dir); err2 != nil {
					warnings = append(warnings, fmt.Sprintf("OpenCode workspace path lookup failed for session %s: %v; %v", id.String, err, err2))
				}
			}
			s.ProjectRoot = dir
		}
		if s.ProjectRoot == "" && projectId.Valid && projectId.String != "" {
			var dir string
			if err := db.QueryRow("SELECT directory FROM project WHERE id = ?", projectId.String).Scan(&dir); err != nil {
				if err2 := db.QueryRow("SELECT path FROM project WHERE id = ?", projectId.String).Scan(&dir); err2 != nil {
					warnings = append(warnings, fmt.Sprintf("OpenCode project path lookup failed for session %s: %v; %v", id.String, err, err2))
				}
			}
			s.ProjectRoot = dir
		}

		// Attempt to count messages
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM message WHERE session_id = ?", id.String).Scan(&count); err != nil {
			if err2 := db.QueryRow("SELECT COUNT(*) FROM message WHERE sessionId = ?", id.String).Scan(&count); err2 != nil {
				warnings = append(warnings, fmt.Sprintf("OpenCode message count lookup failed for session %s: %v; %v", id.String, err, err2))
			}
		}
		s.MessageCount = count

		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed while iterating opencode sessions: %w", err)
	}

	sortSessions(sessions)

	notes := []string{fmt.Sprintf("Read OpenCode sessions from %s", dbPath)}
	notes = append(notes, warnings...)
	return sessions, notes, nil
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return columns, nil
}
