package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// RegistrySource records how a slice came to be managed by slis.
type RegistrySource string

const (
	// SourceCreated is a slice created by `slis create`.
	SourceCreated RegistrySource = "created"
	// SourceImported is a worktree explicitly imported by `slis import`.
	SourceImported RegistrySource = "imported"
	// SourceGrandfathered is a slice registered by the first discovery run on
	// upgrade, so existing users see no behavior change.
	SourceGrandfathered RegistrySource = "grandfathered"
)

// RegistryMember is the recorded git location of one repo within a managed slice.
type RegistryMember struct {
	Branch       string `yaml:"branch"`
	WorktreePath string `yaml:"worktree_path"`
}

// RegistrySlice is one slice slis manages: its per-repo members, when it was
// registered, and how (created / imported / grandfathered).
type RegistrySlice struct {
	Name    string                    `yaml:"name"`
	Members map[string]RegistryMember `yaml:"members"`
	Source  RegistrySource            `yaml:"source"`
	At      time.Time                 `yaml:"at"`
}

// Registry is the persistent record of the slices slis manages. A worktree that
// is not in the registry (and not under the managed worktree tree) is treated as
// a candidate, not silently ingested.
type Registry struct {
	Slices map[string]RegistrySlice `yaml:"slices"`
}

// Import records a worktree as a managed slice member: it creates the slice
// entry if needed (source imported, timestamped) and adds/updates the member for
// the given repo. It never touches git — only this in-memory registry.
func (r *Registry) Import(sliceName, repo, branch, worktreePath string) {
	r.register(sliceName, repo, branch, worktreePath, SourceImported)
}

// RegisterCreated records a worktree created by slis. Recording creation
// immediately (rather than waiting for discovery) makes cleanup and branch
// switches reliable even when an older registry already exists.
func (r *Registry) RegisterCreated(sliceName, repo, branch, worktreePath string) {
	r.register(sliceName, repo, branch, worktreePath, SourceCreated)
}

func (r *Registry) register(sliceName, repo, branch, worktreePath string, source RegistrySource) {
	if r.Slices == nil {
		r.Slices = map[string]RegistrySlice{}
	}
	rs, ok := r.Slices[sliceName]
	if !ok {
		rs = RegistrySlice{
			Name:    sliceName,
			Members: map[string]RegistryMember{},
			Source:  source,
			At:      time.Now().UTC(),
		}
	}
	if rs.Members == nil {
		rs.Members = map[string]RegistryMember{}
	}
	rs.Members[repo] = RegistryMember{Branch: branch, WorktreePath: worktreePath}
	r.Slices[sliceName] = rs
}

// LoadRegistry reads the registry file at path. The returned bool reports
// whether the file existed: a missing file yields an empty registry, exists=false
// and a nil error (the first run grandfathers, using this signal).
func LoadRegistry(path string) (reg Registry, exists bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{Slices: map[string]RegistrySlice{}}, false, nil
		}
		return Registry{}, false, err
	}
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return Registry{}, true, err
	}
	if reg.Slices == nil {
		reg.Slices = map[string]RegistrySlice{}
	}
	return reg, true, nil
}

// SaveRegistry writes reg to path as YAML, creating parent directories as needed.
// The replacement is atomic so an interrupted process cannot leave a truncated
// registry that hides otherwise healthy worktrees on the next launch.
func SaveRegistry(path string, reg Registry) error {
	if reg.Slices == nil {
		reg.Slices = map[string]RegistrySlice{}
	}
	data, err := yaml.Marshal(reg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".slis-registry-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
