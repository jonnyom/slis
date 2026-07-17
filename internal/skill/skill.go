// Package skill installs the embedded slis agent skill into an agent harness's
// skills directory (Claude Code or Codex). The install is idempotent — it only
// rewrites when the skill content changed — and atomic (temp file + rename).
package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Harness names an agent harness slis installs the skill for.
type Harness string

const (
	// Claude installs to ~/.claude/skills/slis (Claude Code).
	Claude Harness = "claude"
	// Codex installs to ~/.agents/skills/slis (the Agent Skills open standard).
	Codex Harness = "codex"
)

// AllHarnesses is the set `--harness both` targets, in stable order.
var AllHarnesses = []Harness{Claude, Codex}

const (
	// repoAgentLink is the AGENT.md link as written in the repo copy of SKILL.md
	// (repo-relative). The installed copy places AGENT.md under references/.
	repoAgentLink      = "../../docs/AGENT.md"
	installedAgentLink = "references/AGENT.md"
)

// TargetHarnesses resolves the --harness flag value to the set of harnesses to
// install for. Empty or "both" means all.
func TargetHarnesses(flag string) ([]Harness, error) {
	switch flag {
	case "", "both":
		return AllHarnesses, nil
	case string(Claude):
		return []Harness{Claude}, nil
	case string(Codex):
		return []Harness{Codex}, nil
	default:
		return nil, fmt.Errorf("unknown harness %q (want claude, codex, or both)", flag)
	}
}

// InstallDir returns the skill install directory for a harness under home.
func InstallDir(home string, h Harness) string {
	switch h {
	case Codex:
		return filepath.Join(home, ".agents", "skills", "slis")
	default:
		return filepath.Join(home, ".claude", "skills", "slis")
	}
}

// ContentVersion is a short content hash of the skill + agent doc, stamped into
// the installed skill's frontmatter so a re-install is a no-op until the content
// actually changes.
func ContentVersion(skillMD, agentDoc string) string {
	sum := sha256.Sum256([]byte(skillMD + "\x00" + agentDoc))
	return hex.EncodeToString(sum[:])[:12]
}

// Frontmatter is the subset of skill YAML frontmatter slis validates.
type Frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// splitFrontmatter separates the leading YAML frontmatter (between "---" fences)
// from the markdown body. ok is false when there is no frontmatter.
func splitFrontmatter(md string) (front, body string, ok bool) {
	const fence = "---\n"
	if !strings.HasPrefix(md, fence) {
		return "", md, false
	}
	rest := md[len(fence):]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", md, false
	}
	front = rest[:idx]
	tail := rest[idx+1:] // starts at the closing "---" fence line
	if nl := strings.Index(tail, "\n"); nl >= 0 {
		body = tail[nl+1:]
	}
	return front, body, true
}

// ParseFrontmatter parses the name/description fields from a skill's frontmatter.
func ParseFrontmatter(md string) (Frontmatter, error) {
	front, _, ok := splitFrontmatter(md)
	if !ok {
		return Frontmatter{}, fmt.Errorf("skill markdown has no YAML frontmatter")
	}
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return Frontmatter{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	return fm, nil
}

// InstalledVersion reads the metadata.version stamped into an installed skill's
// frontmatter, or "" when absent/unparseable.
func InstalledVersion(md string) string {
	front, _, ok := splitFrontmatter(md)
	if !ok {
		return ""
	}
	var stamp struct {
		Metadata struct {
			Version string `yaml:"version"`
		} `yaml:"metadata"`
	}
	if err := yaml.Unmarshal([]byte(front), &stamp); err != nil {
		return ""
	}
	return stamp.Metadata.Version
}

// render produces the installed skill content: the AGENT.md link rewritten to
// references/AGENT.md and metadata.version stamped into the frontmatter.
func render(skillMD, version string) (string, error) {
	front, body, ok := splitFrontmatter(skillMD)
	if !ok {
		return "", fmt.Errorf("skill markdown has no YAML frontmatter")
	}
	var fm map[string]interface{}
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return "", fmt.Errorf("parse frontmatter: %w", err)
	}
	meta, _ := fm["metadata"].(map[string]interface{})
	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta["version"] = version
	fm["metadata"] = meta

	out, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshal frontmatter: %w", err)
	}
	body = strings.ReplaceAll(body, repoAgentLink, installedAgentLink)
	return "---\n" + string(out) + "---\n" + body, nil
}

// Result reports the outcome of an install for one harness.
type Result struct {
	Dir     string
	Changed bool
	Version string
}

// Install writes the skill (SKILL.md) and its reference doc (references/AGENT.md)
// into the harness's skills directory under home. Idempotent: when the installed
// skill already carries the current content version and the reference doc
// matches, nothing is written and Changed is false.
func Install(home string, h Harness, skillMD, agentDoc string) (Result, error) {
	version := ContentVersion(skillMD, agentDoc)
	dir := InstallDir(home, h)
	skillPath := filepath.Join(dir, "SKILL.md")
	refPath := filepath.Join(dir, "references", "AGENT.md")

	rendered, err := render(skillMD, version)
	if err != nil {
		return Result{}, err
	}

	if existing, err := os.ReadFile(skillPath); err == nil && InstalledVersion(string(existing)) == version {
		if ref, err := os.ReadFile(refPath); err == nil && string(ref) == agentDoc {
			return Result{Dir: dir, Changed: false, Version: version}, nil
		}
	}

	if err := writeFileAtomic(refPath, []byte(agentDoc)); err != nil {
		return Result{}, err
	}
	if err := writeFileAtomic(skillPath, []byte(rendered)); err != nil {
		return Result{}, err
	}
	return Result{Dir: dir, Changed: true, Version: version}, nil
}

// writeFileAtomic writes data to path via a sibling temp file + rename (atomic
// on the same filesystem), creating parent directories as needed.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".slis-skill-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename to %s: %w", path, err)
	}
	return nil
}
