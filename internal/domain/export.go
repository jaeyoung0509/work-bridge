package domain

type ExportManifest struct {
	BundleID          string   `json:"bundle_id,omitempty"`
	SourceTool        Tool     `json:"source_tool"`
	SourceSessionID   string   `json:"source_session_id,omitempty"`
	TargetTool        Tool     `json:"target_tool"`
	OutputDir         string   `json:"output_dir"`
	Files             []string `json:"files"`
	PartialFields     []string `json:"partial_fields"`
	UnsupportedFields []string `json:"unsupported_fields"`
	RedactedFields    []string `json:"redacted_fields"`
	Warnings          []string `json:"warnings"`
}
