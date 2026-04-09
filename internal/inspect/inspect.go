package inspect

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/detect"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
)

type Options struct {
	FS        fsx.FS
	CWD       string
	HomeDir   string
	ToolPaths domain.ToolPaths
	Tool      string
	LookPath  func(string) (string, error)
	Limit     int
}

type Report struct {
	Tool          string                 `json:"tool"`
	CWD           string                 `json:"cwd"`
	ProjectRoot   string                 `json:"project_root"`
	Binary        detect.BinaryStatus    `json:"binary"`
	Assets        []detect.ArtifactProbe `json:"assets"`
	Sessions      []Session              `json:"sessions"`
	TotalSessions int                    `json:"total_sessions"`
	Notes         []string               `json:"notes,omitempty"`
}

type Session struct {
	ID           string `json:"id"`
	Title        string `json:"title,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	ProjectRoot  string `json:"project_root,omitempty"`
	StoragePath  string `json:"storage_path,omitempty"`
	MessageCount int    `json:"message_count,omitempty"`
}

type geminiMessage struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content"`
}

var codexRolloutNamePattern = regexp.MustCompile(`^rollout-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-(.+)$`)

func Run(opts Options) (Report, error) {
	if opts.FS == nil {
		return Report{}, errors.New("fs is required")
	}
	if opts.CWD == "" {
		return Report{}, errors.New("cwd is required")
	}
	if opts.HomeDir == "" {
		return Report{}, errors.New("home_dir is required")
	}
	if opts.Tool == "" {
		return Report{}, errors.New("tool is required")
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	detectReport, err := detect.Run(detect.Options{
		FS:        opts.FS,
		CWD:       opts.CWD,
		HomeDir:   opts.HomeDir,
		ToolPaths: opts.ToolPaths,
		LookPath:  opts.LookPath,
	})
	if err != nil {
		return Report{}, err
	}

	var selected *detect.ToolReport
	for i := range detectReport.Tools {
		if detectReport.Tools[i].Tool == opts.Tool {
			selected = &detectReport.Tools[i]
			break
		}
	}
	if selected == nil {
		return Report{}, fmt.Errorf("unsupported tool %q", opts.Tool)
	}

	report := Report{
		Tool:        opts.Tool,
		CWD:         opts.CWD,
		ProjectRoot: detectReport.ProjectRoot,
		Binary:      selected.Binary,
		Assets:      selected.Artifacts,
		Notes:       append([]string{}, selected.Notes...),
	}

	switch opts.Tool {
	case "codex":
		sessions, notes, err := inspectCodex(opts)
		if err != nil {
			return Report{}, err
		}
		report.Sessions = sessions
		report.TotalSessions = len(sessions)
		report.Notes = append(report.Notes, notes...)
	case "gemini":
		sessions, notes, err := inspectGemini(opts)
		if err != nil {
			return Report{}, err
		}
		report.Sessions = sessions
		report.TotalSessions = len(sessions)
		report.Notes = append(report.Notes, notes...)
	case "claude":
		sessions, notes, err := inspectClaude(opts)
		if err != nil {
			return Report{}, err
		}
		report.Sessions = sessions
		report.TotalSessions = len(sessions)
		report.Notes = append(report.Notes, notes...)
	case "opencode":
		sessions, notes, err := inspectOpenCode(opts)
		if err != nil {
			return Report{}, err
		}
		report.Sessions = sessions
		report.TotalSessions = len(sessions)
		report.Notes = append(report.Notes, notes...)
	default:
		return Report{}, fmt.Errorf("unsupported tool %q", opts.Tool)
	}

	if len(report.Sessions) > opts.Limit {
		report.Sessions = report.Sessions[:opts.Limit]
	}

	return report, nil
}

func inspectCodex(opts Options) ([]Session, []string, error) {
	codexDir := opts.ToolPaths.Dir(domain.ToolCodex, opts.HomeDir)
	indexPath := filepath.Join(codexDir, "session_index.jsonl")
	if !pathExists(opts.FS, indexPath) {
		return []Session{}, []string{
			"No Codex session_index.jsonl file was found. Session inventory is unavailable on this machine.",
		}, nil
	}

	pathMap, err := codexSessionPathMap(opts.FS, filepath.Join(codexDir, "sessions"))
	if err != nil {
		return nil, nil, err
	}

	type indexEntry struct {
		ID         string `json:"id"`
		ThreadName string `json:"thread_name"`
		UpdatedAt  string `json:"updated_at"`
	}

	entries, err := readJSONLLines(opts.FS, indexPath, func(line []byte) (indexEntry, error) {
		var entry indexEntry
		err := json.Unmarshal(line, &entry)
		return entry, err
	})
	if err != nil {
		return nil, nil, err
	}

	sessions := make([]Session, 0, len(entries))
	missingPaths := 0
	for _, entry := range entries {
		session := Session{
			ID:        entry.ID,
			Title:     entry.ThreadName,
			UpdatedAt: entry.UpdatedAt,
		}

		if path, ok := pathMap[entry.ID]; ok {
			session.StoragePath = path
			cwd, startedAt, err := readCodexSessionMeta(opts.FS, path)
			if err == nil {
				session.ProjectRoot = cwd
				session.StartedAt = startedAt
			}
		} else {
			missingPaths++
		}

		sessions = append(sessions, session)
	}

	sortSessions(sessions)

	notes := []string{
		"Session inventory comes from ~/.codex/session_index.jsonl and session_meta records in ~/.codex/sessions.",
	}
	if missingPaths > 0 {
		notes = append(notes, fmt.Sprintf("%d Codex sessions were indexed but their backing JSONL file was not found.", missingPaths))
	}

	return sessions, notes, nil
}

func inspectGemini(opts Options) ([]Session, []string, error) {
	geminiDir := opts.ToolPaths.Dir(domain.ToolGemini, opts.HomeDir)
	projectsPath := filepath.Join(geminiDir, "projects.json")
	type projectsFile struct {
		Projects map[string]string `json:"projects"`
	}

	projectData := projectsFile{Projects: map[string]string{}}
	if data, err := opts.FS.ReadFile(projectsPath); err == nil {
		_ = json.Unmarshal(data, &projectData)
	}

	aliasToProject := make(map[string]string, len(projectData.Projects))
	for projectRoot, alias := range projectData.Projects {
		aliasToProject[alias] = projectRoot
	}

	files, err := listFilesRecursive(opts.FS, filepath.Join(geminiDir, "tmp"))
	if err != nil {
		return nil, nil, err
	}

	type geminiSession struct {
		SessionID   string          `json:"sessionId"`
		StartTime   string          `json:"startTime"`
		LastUpdated string          `json:"lastUpdated"`
		Messages    []geminiMessage `json:"messages"`
	}

	sessions := []Session{}
	unmappedProjects := 0
	malformedSessions := 0
	unreadableSessions := 0

	for _, path := range files {
		if filepath.Ext(path) != ".json" || !strings.Contains(path, string(filepath.Separator)+"chats"+string(filepath.Separator)+"session-") {
			continue
		}

		data, err := opts.FS.ReadFile(path)
		if err != nil {
			unreadableSessions++
			continue
		}

		var raw geminiSession
		if err := json.Unmarshal(data, &raw); err != nil {
			malformedSessions++
			continue
		}

		alias := filepath.Base(filepath.Dir(filepath.Dir(path)))
		projectRoot := aliasToProject[alias]
		if projectRoot == "" {
			unmappedProjects++
		}

		sessions = append(sessions, Session{
			ID:           raw.SessionID,
			Title:        geminiSessionTitle(raw.Messages),
			StartedAt:    raw.StartTime,
			UpdatedAt:    raw.LastUpdated,
			ProjectRoot:  projectRoot,
			StoragePath:  path,
			MessageCount: len(raw.Messages),
		})
	}

	sortSessions(sessions)

	notes := []string{
		"Session inventory comes from ~/.gemini/projects.json and ~/.gemini/tmp/*/chats/session-*.json.",
	}
	if unmappedProjects > 0 {
		notes = append(notes, fmt.Sprintf("%d Gemini sessions did not map back to a known project root from projects.json.", unmappedProjects))
	}
	if malformedSessions > 0 {
		notes = append(notes, fmt.Sprintf("%d Gemini session files were skipped because they could not be parsed.", malformedSessions))
	}
	if unreadableSessions > 0 {
		notes = append(notes, fmt.Sprintf("%d Gemini session files were skipped because they could not be read.", unreadableSessions))
	}

	return sessions, notes, nil
}

func inspectClaude(opts Options) ([]Session, []string, error) {
	claudeDir := opts.ToolPaths.Dir(domain.ToolClaude, opts.HomeDir)
	historyPath := filepath.Join(claudeDir, "history.jsonl")
	if !pathExists(opts.FS, historyPath) {
		return []Session{}, []string{
			"No Claude history.jsonl file was found. Session inventory is unavailable on this machine.",
		}, nil
	}

	type historyEntry struct {
		Display   string `json:"display"`
		Timestamp int64  `json:"timestamp"`
		Project   string `json:"project"`
		SessionID string `json:"sessionId"`
	}

	type aggregate struct {
		Session
		minTS int64
		maxTS int64
	}

	entries, err := readJSONLLines(opts.FS, historyPath, func(line []byte) (historyEntry, error) {
		var entry historyEntry
		err := json.Unmarshal(line, &entry)
		return entry, err
	})
	if err != nil {
		return nil, nil, err
	}

	bySession := map[string]*aggregate{}
	for _, entry := range entries {
		if entry.SessionID == "" {
			continue
		}

		current, ok := bySession[entry.SessionID]
		if !ok {
			current = &aggregate{
				Session: Session{
					ID:          entry.SessionID,
					ProjectRoot: entry.Project,
					StoragePath: historyPath,
				},
				minTS: entry.Timestamp,
				maxTS: entry.Timestamp,
			}
			bySession[entry.SessionID] = current
		}

		if entry.Project != "" {
			current.ProjectRoot = entry.Project
		}
		if entry.Timestamp < current.minTS {
			current.minTS = entry.Timestamp
		}
		if entry.Timestamp > current.maxTS {
			current.maxTS = entry.Timestamp
			if entry.Display != "" {
				current.Title = entry.Display
			}
		}
		if current.Title == "" && entry.Display != "" {
			current.Title = entry.Display
		}
		current.MessageCount++
	}

	sessions := make([]Session, 0, len(bySession))
	for _, session := range bySession {
		session.StartedAt = epochMillisToRFC3339(session.minTS)
		session.UpdatedAt = epochMillisToRFC3339(session.maxTS)
		sessions = append(sessions, session.Session)
	}

	sortSessions(sessions)

	notes := []string{
		"Session inventory comes from ~/.claude/history.jsonl. This is a best-effort history-based view, not a full raw session export.",
	}

	return sessions, notes, nil
}

func inspectOpenCode(opts Options) ([]Session, []string, error) {
	opencodeDir := opts.ToolPaths.Dir(domain.ToolOpenCode, opts.HomeDir)
	roots := []string{
		filepath.Join(opencodeDir, "storage", "session"),
		filepath.Join(opencodeDir, "project"),
		filepath.Join(opts.HomeDir, ".config", "opencode"),
	}

	files := []string{}
	for _, root := range roots {
		candidates, err := listFilesRecursive(opts.FS, root)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, candidates...)
	}

	sessions := []Session{}
	seen := map[string]struct{}{}
	unreadable := 0
	unparsed := 0

	for _, path := range files {
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".jsonl" && ext != ".jsonc" {
			continue
		}

		data, err := opts.FS.ReadFile(path)
		if err != nil {
			unreadable++
			continue
		}

		session, ok := parseOpenCodeSession(path, data)
		if !ok {
			unparsed++
			continue
		}
		if session.ID == "" {
			session.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		if _, exists := seen[session.ID]; exists {
			continue
		}
		seen[session.ID] = struct{}{}
		sessions = append(sessions, session)
	}

	sortSessions(sessions)

	notes := []string{
		"OpenCode sessions are discovered from ~/.local/share/opencode/storage/session and related project storage directories.",
	}
	if unreadable > 0 {
		notes = append(notes, fmt.Sprintf("%d OpenCode session files could not be read.", unreadable))
	}
	if unparsed > 0 {
		notes = append(notes, fmt.Sprintf("%d OpenCode files looked session-like but could not be parsed.", unparsed))
	}
	return sessions, notes, nil
}

func parseOpenCodeSession(path string, data []byte) (Session, bool) {
	if strings.HasSuffix(strings.ToLower(path), ".jsonl") {
		lines := splitLines(data)
		for _, line := range lines {
			session, ok := parseOpenCodeSessionJSON(path, line)
			if ok {
				return session, true
			}
		}
		return Session{}, false
	}
	return parseOpenCodeSessionJSON(path, data)
}

func splitLines(data []byte) [][]byte {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	lines := [][]byte{}
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		lines = append(lines, append([]byte(nil), line...))
	}
	return lines
}

func parseOpenCodeSessionJSON(path string, data []byte) (Session, bool) {
	cleaned := stripJSONCComments(data)
	var raw map[string]any
	if err := json.Unmarshal(cleaned, &raw); err != nil {
		return Session{}, false
	}

	session := Session{
		StoragePath: path,
		ProjectRoot: stringField(raw, "projectRoot", "project_root", "cwd", "dir", "path"),
	}
	session.ID = stringField(raw, "id", "sessionId", "sessionID", "session_id")
	session.Title = stringField(raw, "title", "display", "thread_name", "prompt", "name")
	session.StartedAt = stringField(raw, "createdAt", "created_at", "startTime", "start_time", "startedAt", "started_at", "timestamp")
	session.UpdatedAt = stringField(raw, "updatedAt", "updated_at", "lastUpdated", "last_updated", "modifiedAt", "modified_at")
	if session.ProjectRoot == "" {
		session.ProjectRoot = stringField(raw, "projectID", "projectId")
	}
	if session.Title == "" {
		session.Title = summarizeOpenCodeTitle(raw)
	}
	if count := len(anySlice(raw, "messages", "events", "items")); count > 0 {
		session.MessageCount = count
	}
	if session.ID == "" && session.Title == "" && session.ProjectRoot == "" {
		return Session{}, false
	}
	return session, true
}

func summarizeOpenCodeTitle(raw map[string]any) string {
	for _, key := range []string{"prompt", "summary", "display", "title"} {
		if value := stringField(raw, key); value != "" {
			return truncate(value)
		}
	}
	return ""
}

func stringField(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func anySlice(raw map[string]any, keys ...string) []any {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if slice, ok := value.([]any); ok {
			return slice
		}
	}
	return nil
}

func stripJSONCComments(data []byte) []byte {
	lines := splitLines(data)
	if len(lines) == 0 {
		return data
	}
	var b strings.Builder
	inBlock := false
	for _, line := range lines {
		text := string(line)
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		if inBlock {
			if end := strings.Index(text, "*/"); end >= 0 {
				inBlock = false
				text = text[end+2:]
			} else {
				continue
			}
		}
		for {
			start := strings.Index(text, "/*")
			if start < 0 {
				break
			}
			end := strings.Index(text[start+2:], "*/")
			if end < 0 {
				text = text[:start]
				inBlock = true
				break
			}
			text = text[:start] + text[start+2+end+2:]
		}
		if idx := strings.Index(text, "//"); idx >= 0 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		text = strings.TrimRight(text, ",")
		b.WriteString(text)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func readCodexSessionMeta(fs fsx.FS, path string) (string, string, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	line, err := firstLine(data)
	if err != nil {
		return "", "", err
	}

	var payload struct {
		Type    string `json:"type"`
		Payload struct {
			Timestamp string `json:"timestamp"`
			CWD       string `json:"cwd"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(line, &payload); err != nil {
		return "", "", err
	}

	return payload.Payload.CWD, payload.Payload.Timestamp, nil
}

func geminiSessionTitle(messages []geminiMessage) string {
	for _, message := range messages {
		if message.Type != "user" {
			continue
		}
		title := parseGeminiContent(message.Content)
		if title != "" {
			return title
		}
	}
	return ""
}

func parseGeminiContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return truncate(text)
	}

	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		for _, block := range blocks {
			if block.Text != "" {
				return truncate(block.Text)
			}
		}
	}

	return ""
}

func truncate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 80 {
		return value
	}
	return value[:77] + "..."
}

func sortSessions(sessions []Session) {
	sort.Slice(sessions, func(i, j int) bool {
		left := parseTime(sessions[i].UpdatedAt)
		right := parseTime(sessions[j].UpdatedAt)
		if left.Equal(right) {
			return sessions[i].ID > sessions[j].ID
		}
		return left.After(right)
	})
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func epochMillisToRFC3339(value int64) string {
	if value == 0 {
		return ""
	}
	return time.UnixMilli(value).UTC().Format(time.RFC3339)
}

func codexSessionPathMap(fs fsx.FS, root string) (map[string]string, error) {
	files, err := listFilesRecursive(fs, root)
	if err != nil {
		return nil, err
	}

	mapped := map[string]string{}
	for _, path := range files {
		if filepath.Ext(path) != ".jsonl" {
			continue
		}

		if sessionID, err := readCodexSessionID(fs, path); err == nil && sessionID != "" {
			mapped[sessionID] = path
			continue
		}

		base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		matches := codexRolloutNamePattern.FindStringSubmatch(base)
		if len(matches) == 2 && matches[1] != "" {
			mapped[matches[1]] = path
		}
	}

	return mapped, nil
}

func listFilesRecursive(fs fsx.FS, root string) ([]string, error) {
	info, err := fs.Stat(root)
	if err != nil {
		return nil, nil
	}
	if !info.IsDir() {
		return []string{root}, nil
	}

	type item struct {
		path string
	}
	queue := []item{{path: root}}
	files := []string{}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		entries, err := fs.ReadDir(current.path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			path := filepath.Join(current.path, entry.Name())
			if entry.IsDir() {
				queue = append(queue, item{path: path})
				continue
			}
			files = append(files, path)
		}
	}

	sort.Strings(files)
	return files, nil
}

func firstLine(data []byte) ([]byte, error) {
	reader := bufio.NewReader(strings.NewReader(string(data)))
	line, err := reader.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return bytesTrimSpace(line), nil
}

func readCodexSessionID(fs fsx.FS, path string) (string, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return "", err
	}

	line, err := firstLine(data)
	if err != nil {
		return "", err
	}

	var payload struct {
		Type    string `json:"type"`
		Payload struct {
			ID string `json:"id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(line, &payload); err != nil {
		return "", err
	}
	if payload.Type != "session_meta" {
		return "", fmt.Errorf("unexpected codex record type %q", payload.Type)
	}

	return payload.Payload.ID, nil
}

func bytesTrimSpace(data []byte) []byte {
	return []byte(strings.TrimSpace(string(data)))
}

func pathExists(fs fsx.FS, path string) bool {
	_, err := fs.Stat(path)
	return err == nil
}

func readJSONLLines[T any](fs fsx.FS, path string, parse func([]byte) (T, error)) ([]T, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 8*1024*1024)

	values := []T{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		value, err := parse([]byte(line))
		if err != nil {
			continue
		}
		values = append(values, value)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return values, nil
}
