package discovery

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
)

// Overrides maps slice name -> repo -> branch. An entry forces that repo's
// worktree (identified by branch) into the named slice, regardless of how
// branch-name auto-grouping would have placed it.
type Overrides map[string]map[string]string

// Folded maps slice name -> repo -> the branches a gather subsumed into that
// slice's tip. A folded branch is hidden as a standalone slice (its commits live
// in the gathered slice's representative tip); its worktree is never touched.
type Folded map[string]map[string][]string

// overridesFile is the on-disk YAML shape.
type overridesFile struct {
	Overrides Overrides `yaml:"overrides"`
	Folded    Folded    `yaml:"folded,omitempty"`
}

// ApplyFolds drops members whose branch has been folded into a gathered slice's
// tip, then removes any slice left empty. Run it after Apply: the tip stays (it
// is a normal override member), only the subsumed intermediates disappear. The
// returned slice is sorted by name.
func ApplyFolds(slices []model.Slice, folded Folded) []model.Slice {
	if len(folded) == 0 {
		return slices
	}
	hidden := make(map[string]bool)
	for _, repoToBranches := range folded {
		for repo, branches := range repoToBranches {
			for _, b := range branches {
				hidden[repo+"\x00"+b] = true
			}
		}
	}

	out := make([]model.Slice, 0, len(slices))
	for _, s := range slices {
		kept := make(map[string]model.SliceMember, len(s.Members))
		for repo, m := range s.Members {
			if hidden[m.Repo+"\x00"+m.Branch] {
				continue
			}
			kept[repo] = m
		}
		if len(kept) == 0 {
			continue
		}
		s.Members = kept
		out = append(out, s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
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

// Resolve applies both the grouping overrides and the gather folds stored at
// overridesPath to the discovered slices: overrides regroup members, folds hide
// branches a gather subsumed into a tip. This is the standard read-path view.
func Resolve(slices []model.Slice, overridesPath, stripPrefix string) []model.Slice {
	ov, _ := LoadOverrides(overridesPath)
	folded, _ := LoadFolded(overridesPath)
	retargetMovedOverrideMembers(slices, ov, stripPrefix)
	return ApplyFolds(Apply(slices, ov), folded)
}

func retargetMovedOverrideMembers(slices []model.Slice, overrides Overrides, stripPrefix string) {
	branches := make(map[string]map[string]bool)
	byName := make(map[string]model.Slice)
	for _, slice := range slices {
		byName[slice.Name] = slice
		for _, member := range slice.Members {
			if branches[member.Repo] == nil {
				branches[member.Repo] = make(map[string]bool)
			}
			branches[member.Repo][member.Branch] = true
		}
	}
	for _, repoBranches := range overrides {
		for repo, branch := range repoBranches {
			if branches[repo][branch] {
				continue
			}
			source, ok := byName[config.SliceNameFromBranch(branch, stripPrefix)]
			if !ok {
				continue
			}
			member, ok := source.Members[repo]
			if ok {
				repoBranches[repo] = member.Branch
			}
		}
	}
}

// SaveOverrides writes ov to the file at path as YAML, preserving any existing
// folded section (so callers that only touch groupings don't drop gathers). The
// parent directory is created if it does not exist.
func SaveOverrides(path string, ov Overrides) error {
	folded, _ := LoadFolded(path)
	return SaveConfig(path, ov, folded)
}

// SaveConfig writes both the overrides and folded sections to path atomically.
// Use it when a mutation touches both (gather/scatter).
func SaveConfig(path string, ov Overrides, folded Folded) error {
	data, err := yaml.Marshal(overridesFile{Overrides: ov, Folded: folded})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadFolded reads the folded section of the file at path. A missing file (or
// missing section) yields an empty Folded and a nil error.
func LoadFolded(path string) (Folded, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Folded{}, nil
		}
		return nil, err
	}
	var f overridesFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Folded == nil {
		return Folded{}, nil
	}
	return f.Folded, nil
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
