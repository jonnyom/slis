package discovery

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/jonnyom/slis/internal/model"
)

// Overrides maps slice name -> repo -> branch. An entry forces that repo's
// worktree (identified by branch) into the named slice, regardless of how
// branch-name auto-grouping would have placed it.
type Overrides map[string]map[string]string

// overridesFile is the on-disk YAML shape.
type overridesFile struct {
	Overrides Overrides `yaml:"overrides"`
}

// Apply rebuilds slice membership according to the given overrides.
//
// For every (sliceName, repo, branch) in ov, the corresponding discovered
// member is moved into sliceName. Members not mentioned by any override
// remain in their auto-grouped slice. Slices that end up empty are dropped.
// The returned slice is sorted by name.
func Apply(slices []model.Slice, ov Overrides) []model.Slice {
	// Build lookup: repo -> branch -> member.
	lookup := make(map[string]map[string]model.SliceMember)
	for _, s := range slices {
		for _, m := range s.Members {
			if _, ok := lookup[m.Repo]; !ok {
				lookup[m.Repo] = make(map[string]model.SliceMember)
			}
			lookup[m.Repo][m.Branch] = m
		}
	}

	// claimed tracks members consumed by an override so they are not also
	// added in the second pass.
	claimed := make(map[string]bool)

	result := make(map[string]*model.Slice)

	lazySlice := func(name string) *model.Slice {
		if result[name] == nil {
			result[name] = &model.Slice{
				Name:    name,
				Members: make(map[string]model.SliceMember),
			}
		}
		return result[name]
	}

	// First pass: apply overrides.
	for sliceName, repoToBranch := range ov {
		for repo, branch := range repoToBranch {
			byBranch, ok := lookup[repo]
			if !ok {
				continue
			}
			member, ok := byBranch[branch]
			if !ok {
				continue
			}
			lazySlice(sliceName).Members[repo] = member
			claimed[repo+"\x00"+branch] = true
		}
	}

	// Second pass: unclaimed members stay in their original slices.
	for _, s := range slices {
		for _, m := range s.Members {
			if claimed[m.Repo+"\x00"+m.Branch] {
				continue
			}
			lazySlice(s.Name).Members[m.Repo] = m
		}
	}

	// Collect, drop empty, sort.
	keys := make([]string, 0, len(result))
	for k, sl := range result {
		if len(sl.Members) > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	out := make([]model.Slice, len(keys))
	for i, k := range keys {
		out[i] = *result[k]
	}
	return out
}

// SaveOverrides writes ov to the file at path as YAML. The parent directory is
// created if it does not exist.
func SaveOverrides(path string, ov Overrides) error {
	data, err := yaml.Marshal(overridesFile{Overrides: ov})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadOverrides reads the overrides file at path. If the file does not exist
// it returns an empty Overrides and a nil error (no overrides is normal).
func LoadOverrides(path string) (Overrides, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Overrides{}, nil
		}
		return nil, err
	}
	var f overridesFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Overrides == nil {
		f.Overrides = Overrides{}
	}
	return f.Overrides, nil
}
