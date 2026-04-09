package packagex

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"sessionport/internal/doctor"
	"sessionport/internal/domain"
	"sessionport/internal/exporter"
	"sessionport/internal/platform/archivex"
	"sessionport/internal/platform/fsx"
)

const (
	bundleFileName   = "bundle.json"
	manifestFileName = "spkg-manifest.json"
	archiveVersion   = "spkg/v0"
)

type PackOptions struct {
	Archive archivex.Writer
	Bundle  domain.SessionBundle
	OutFile string
}

type UnpackOptions struct {
	Archive archivex.Reader
	FS      fsx.FS
	File    string
	Target  domain.Tool
	OutDir  string
}

func Pack(opts PackOptions) (domain.PackageManifest, error) {
	if opts.Archive == nil {
		return domain.PackageManifest{}, errors.New("archive writer is required")
	}
	if opts.OutFile == "" {
		return domain.PackageManifest{}, errors.New("out_file is required")
	}
	if opts.Bundle.AssetKind == "" {
		opts.Bundle.AssetKind = domain.AssetKindSession
	}
	if err := opts.Bundle.Validate(); err != nil {
		return domain.PackageManifest{}, err
	}

	bundleData, err := json.MarshalIndent(opts.Bundle, "", "  ")
	if err != nil {
		return domain.PackageManifest{}, err
	}

	manifest := domain.PackageManifest{
		ArchiveVersion:  archiveVersion,
		AssetKind:       opts.Bundle.AssetKind,
		BundleID:        opts.Bundle.BundleID,
		SourceTool:      opts.Bundle.SourceTool,
		SourceSessionID: opts.Bundle.SourceSessionID,
		Files:           []string{bundleFileName, manifestFileName},
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return domain.PackageManifest{}, err
	}

	files := []archivex.File{
		{Name: bundleFileName, Body: append(bundleData, '\n')},
		{Name: manifestFileName, Body: append(manifestData, '\n')},
	}
	if err := opts.Archive.WritePackage(opts.OutFile, files); err != nil {
		return domain.PackageManifest{}, err
	}

	return manifest, nil
}

func Unpack(opts UnpackOptions) (domain.UnpackResult, error) {
	if opts.Archive == nil {
		return domain.UnpackResult{}, errors.New("archive reader is required")
	}
	if opts.FS == nil {
		return domain.UnpackResult{}, errors.New("fs is required")
	}
	if opts.File == "" {
		return domain.UnpackResult{}, errors.New("file is required")
	}
	if opts.OutDir == "" {
		return domain.UnpackResult{}, errors.New("out_dir is required")
	}
	if !opts.Target.IsKnown() {
		return domain.UnpackResult{}, fmt.Errorf("unsupported target tool %q", opts.Target)
	}

	files, err := opts.Archive.ReadPackage(opts.File)
	if err != nil {
		return domain.UnpackResult{}, err
	}

	byName := map[string][]byte{}
	for _, file := range files {
		byName[file.Name] = file.Body
	}

	bundleData, ok := byName[bundleFileName]
	if !ok {
		return domain.UnpackResult{}, errors.New("bundle.json is missing from archive")
	}
	manifestData, ok := byName[manifestFileName]
	if !ok {
		return domain.UnpackResult{}, errors.New("spkg-manifest.json is missing from archive")
	}

	var bundle domain.SessionBundle
	if err := json.Unmarshal(bundleData, &bundle); err != nil {
		return domain.UnpackResult{}, err
	}
	if bundle.AssetKind == "" {
		bundle.AssetKind = domain.AssetKindSession
	}
	if err := bundle.Validate(); err != nil {
		return domain.UnpackResult{}, err
	}

	var manifest domain.PackageManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return domain.UnpackResult{}, err
	}

	if err := opts.FS.MkdirAll(opts.OutDir, 0o755); err != nil {
		return domain.UnpackResult{}, err
	}
	bundlePath := filepath.Join(opts.OutDir, bundleFileName)
	if err := opts.FS.WriteFile(bundlePath, bundleData, 0o644); err != nil {
		return domain.UnpackResult{}, err
	}
	if err := opts.FS.WriteFile(filepath.Join(opts.OutDir, manifestFileName), manifestData, 0o644); err != nil {
		return domain.UnpackResult{}, err
	}

	report, err := doctor.Analyze(doctor.Options{
		Bundle: bundle,
		Target: opts.Target,
	})
	if err != nil {
		return domain.UnpackResult{}, err
	}

	exportManifest, err := exporter.Export(exporter.Options{
		FS:     opts.FS,
		Bundle: bundle,
		Report: report,
		OutDir: opts.OutDir,
	})
	if err != nil {
		return domain.UnpackResult{}, err
	}

	return domain.UnpackResult{
		BundlePath:      bundlePath,
		PackageManifest: manifest,
		ExportManifest:  exportManifest,
	}, nil
}
