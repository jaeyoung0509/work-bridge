package importer

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func importOpenCodeSQLite(dbPath string, sessionID string, raw *RawImportResult) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open opencode db: %w", err)
	}
	defer db.Close()

	// 1. Fetch Session fields (project_id, workspace_id, timestamp)
	// Try snake_case first
	var projID, wsID, ts sql.NullString
	if err := db.QueryRow("SELECT project_id, workspace_id, updated_at FROM session WHERE id = ?", sessionID).Scan(&projID, &wsID, &ts); err != nil {
		// Fallback to camelCase
		db.QueryRow("SELECT projectId, workspaceId, updatedAt FROM session WHERE id = ?", sessionID).Scan(&projID, &wsID, &ts)
	}

	if raw.ProjectRoot == "" {
		if wsID.Valid && wsID.String != "" {
			var dir string
			if err := db.QueryRow("SELECT directory FROM workspace WHERE id = ?", wsID.String).Scan(&dir); err == nil {
				raw.ProjectRoot = dir
			} else if err := db.QueryRow("SELECT path FROM workspace WHERE id = ?", wsID.String).Scan(&dir); err == nil {
				raw.ProjectRoot = dir
			}
		}
		if raw.ProjectRoot == "" && projID.Valid && projID.String != "" {
			var dir string
			if err := db.QueryRow("SELECT directory FROM project WHERE id = ?", projID.String).Scan(&dir); err == nil {
				raw.ProjectRoot = dir
			} else if err := db.QueryRow("SELECT path FROM project WHERE id = ?", projID.String).Scan(&dir); err == nil {
				raw.ProjectRoot = dir
			}
		}
	}

	// 2. Fetch Messages
	var msgQuery = "SELECT id, text FROM message WHERE session_id = ?"
	var useCamel bool
	if _, err := db.Query("SELECT id FROM message WHERE sessionId = ? LIMIT 1", sessionID); err == nil {
		msgQuery = "SELECT id, text FROM message WHERE sessionId = ?"
		useCamel = true
	} else if _, err := db.Query("SELECT id FROM message WHERE session_id = ? LIMIT 1", sessionID); err != nil {
		// neither works, returning
		return nil
	}

	rows, err := db.Query(msgQuery, sessionID)
	if err != nil {
		// try content instead of text
		if useCamel {
			msgQuery = "SELECT id, content FROM message WHERE sessionId = ?"
		} else {
			msgQuery = "SELECT id, content FROM message WHERE session_id = ?"
		}
		rows, err = db.Query(msgQuery, sessionID)
		if err != nil {
			return nil
		}
	}
	defer rows.Close()

	var msgCount int
	for rows.Next() {
		msgCount++
		var msgID, msgText sql.NullString
		if err := rows.Scan(&msgID, &msgText); err != nil {
			continue
		}

		if raw.Summary == "" && msgText.Valid && msgText.String != "" {
			raw.Summary = truncateText(msgText.String, 160)
		}
		if msgText.Valid {
			addNarrativeSignals(raw, msgText.String, dbPath)
		}

		if msgID.Valid {
			partQuery := "SELECT text FROM part WHERE message_id = ?"
			if useCamel {
				partQuery = "SELECT text FROM part WHERE messageId = ?"
			}
			partRows, err := db.Query(partQuery, msgID.String)
			if err != nil {
				if useCamel {
					partQuery = "SELECT content FROM part WHERE messageId = ?"
				} else {
					partQuery = "SELECT content FROM part WHERE message_id = ?"
				}
				partRows, err = db.Query(partQuery, msgID.String)
			}
			if err == nil {
				for partRows.Next() {
					var partText sql.NullString
					if err := partRows.Scan(&partText); err == nil && partText.Valid {
						if raw.Summary == "" && partText.String != "" {
							raw.Summary = truncateText(partText.String, 160)
						}
						addNarrativeSignals(raw, partText.String, dbPath)
					}
				}
				partRows.Close()
			}
		}
	}

	raw.ResumeHints = append(raw.ResumeHints, fmt.Sprintf("message_count=%d", msgCount))
	return nil
}
