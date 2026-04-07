package domain

type PackageManifest struct {
	ArchiveVersion  string   `json:"archive_version"`
	BundleID        string   `json:"bundle_id,omitempty"`
	SourceTool      Tool     `json:"source_tool"`
	SourceSessionID string   `json:"source_session_id,omitempty"`
	Files           []string `json:"files"`
}

type UnpackResult struct {
	BundlePath      string          `json:"bundle_path"`
	PackageManifest PackageManifest `json:"package_manifest"`
	ExportManifest  ExportManifest  `json:"export_manifest"`
}
