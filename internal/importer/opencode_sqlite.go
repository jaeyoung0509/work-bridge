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
	defer func() {
		_ = db.Close()
	}()

	// 1. Fetch Session fields (project_id, workspace_id, timestamp)
	// Try snake_case first
	var projID, wsID, ts sql.NullString
	if err := db.QueryRow("SELECT project_id, workspace_id, updated_at FROM session WHERE id = ?", sessionID).Scan(&projID, &wsID, &ts); err != nil {
		if err2 := db.QueryRow("SELECT projectId, workspaceId, updatedAt FROM session WHERE id = ?", sessionID).Scan(&projID, &wsID, &ts); err2 != nil {
			return fmt.Errorf("query session metadata: snake_case: %v; camelCase: %w", err, err2)
		}
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
	if probeRows, err := db.Query("SELECT id FROM message WHERE sessionId = ? LIMIT 1", sessionID); err == nil {
		_ = probeRows.Close()
		msgQuery = "SELECT id, text FROM message WHERE sessionId = ?"
		useCamel = true
	} else {
		probeRows, probeErr := db.Query("SELECT id FROM message WHERE session_id = ? LIMIT 1", sessionID)
		if probeErr != nil {
			return fmt.Errorf("probe message schema: camelCase: %v; snake_case: %w", err, probeErr)
		}
		_ = probeRows.Close()
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
			return fmt.Errorf("query message rows: %w", err)
		}
	}
	defer func() {
		_ = rows.Close()
	}()

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
				if err := partRows.Err(); err != nil {
					_ = partRows.Close()
					return fmt.Errorf("iterating part rows: %w", err)
				}
				if err := partRows.Close(); err != nil {
					return fmt.Errorf("closing part rows: %w", err)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating message rows: %w", err)
	}

	raw.ResumeHints = append(raw.ResumeHints, fmt.Sprintf("message_count=%d", msgCount))
	return nil
}
