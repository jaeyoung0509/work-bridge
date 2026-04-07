package domain

type CompatibilityReport struct {
	BundleID           string   `json:"bundle_id,omitempty"`
	SourceTool         Tool     `json:"source_tool"`
	SourceSessionID    string   `json:"source_session_id,omitempty"`
	ProjectRoot        string   `json:"project_root,omitempty"`
	TargetTool         Tool     `json:"target_tool"`
	CompatibleFields   []string `json:"compatible_fields"`
	PartialFields      []string `json:"partial_fields"`
	UnsupportedFields  []string `json:"unsupported_fields"`
	RedactedFields     []string `json:"redacted_fields"`
	GeneratedArtifacts []string `json:"generated_artifacts"`
	Warnings           []string `json:"warnings"`
}
