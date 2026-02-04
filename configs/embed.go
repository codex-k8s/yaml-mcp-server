package configs

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed *.yaml
var embeddedConfigs embed.FS

// Names returns the list of embedded YAML config filenames.
func Names() []string {
	entries, err := fs.Glob(embeddedConfigs, "*.yaml")
	if err != nil {
		return nil
	}
	sort.Strings(entries)
	return entries
}

// Load returns the embedded YAML config by filename.
func Load(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("embedded config name is empty")
	}
	data, err := fs.ReadFile(embeddedConfigs, name)
	if err != nil {
		return nil, fmt.Errorf("read embedded config %q: %w", name, err)
	}
	return data, nil
}
