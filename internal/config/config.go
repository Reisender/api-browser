// Package config handles persisted connection profiles.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Reisender/api-browser/internal/auth"
	"gopkg.in/yaml.v3"
)

// Profile is a saved connection: a base URL, a spec and auth settings.
type Profile struct {
	Name    string            `yaml:"name"`
	BaseURL string            `yaml:"baseUrl"`
	Spec    string            `yaml:"spec"` // builtin name or file path
	Auth    auth.Config       `yaml:"auth"`
	Headers map[string]string `yaml:"headers,omitempty"` // extra static headers
}

// File is the on-disk config document.
type File struct {
	Default  string    `yaml:"default,omitempty"`
	Profiles []Profile `yaml:"profiles"`
}

// DefaultPath returns the default config file location.
func DefaultPath() string {
	if p := os.Getenv("APIBROWSER_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "api-browser", "config.yaml")
}

// Load reads a config file. A missing file yields an empty File, not an error.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &File{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &f, nil
}

// Save writes the config file, creating parent directories as needed.
func (f *File) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Get returns the named profile, or the default if name is empty.
func (f *File) Get(name string) (*Profile, bool) {
	if name == "" {
		name = f.Default
	}
	for i := range f.Profiles {
		if f.Profiles[i].Name == name {
			return &f.Profiles[i], true
		}
	}
	return nil, false
}

// Put inserts or replaces a profile by name.
func (f *File) Put(p Profile) {
	for i := range f.Profiles {
		if f.Profiles[i].Name == p.Name {
			f.Profiles[i] = p
			return
		}
	}
	f.Profiles = append(f.Profiles, p)
	sort.Slice(f.Profiles, func(i, j int) bool { return f.Profiles[i].Name < f.Profiles[j].Name })
}
