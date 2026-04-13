package importer

import (
	"fmt"
	"sync"
)

type ToolImporter interface {
	ImportRaw(opts Options) (RawImportResult, error)
}

var (
	importersMu sync.RWMutex
	importers   = make(map[string]ToolImporter)
)

func Register(tool string, importer ToolImporter) {
	importersMu.Lock()
	defer importersMu.Unlock()
	if importer == nil {
		panic("importer: Register importer is nil")
	}
	if _, dup := importers[tool]; dup {
		panic("importer: Register called twice for tool " + tool)
	}
	importers[tool] = importer
}

func getImporter(tool string) (ToolImporter, error) {
	importersMu.RLock()
	defer importersMu.RUnlock()
	importer, ok := importers[tool]
	if !ok {
		return nil, fmt.Errorf("unsupported tool %q", tool)
	}
	return importer, nil
}
