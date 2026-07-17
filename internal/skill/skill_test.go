package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleSkill = `---
name: slis
description: A test skill.
---

# slis

Full schemas live in [` + "`docs/AGENT.md`" + `](../../docs/AGENT.md). Read it first.
`

const sampleAgentDoc = "# Driving slis with agents\n\nThe contract.\n"

func TestInstallWritesSkillAndReference(t *testing.T) {
	home := t.TempDir()

	res, err := Install(home, Claude, sampleSkill, sampleAgentDoc)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !res.Changed {
		t.Error("first install should report Changed=true")
	}
	if want := filepath.Join(home, ".claude", "skills", "slis"); res.Dir != want {
		t.Errorf("dir = %q, want %q", res.Dir, want)
	}

	skillData, err := os.ReadFile(filepath.Join(res.Dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed SKILL.md: %v", err)
	}
	got := string(skillData)

	// The repo-relative AGENT.md link must be rewritten to references/AGENT.md,
	// and the original repo link must be gone.
	if !strings.Contains(got, installedAgentLink) {
		t.Errorf("installed skill missing rewritten link %q:\n%s", installedAgentLink, got)
	}
	if strings.Contains(got, repoAgentLink) {
		t.Errorf("installed skill still has repo link %q:\n%s", repoAgentLink, got)
	}

	// The version must be stamped into the frontmatter.
	if InstalledVersion(got) != res.Version {
		t.Errorf("stamped version = %q, want %q", InstalledVersion(got), res.Version)
	}

	// The reference doc must be written verbatim.
	refData, err := os.ReadFile(filepath.Join(res.Dir, "references", "AGENT.md"))
	if err != nil {
		t.Fatalf("read installed reference: %v", err)
	}
	if string(refData) != sampleAgentDoc {
		t.Errorf("reference doc = %q, want %q", string(refData), sampleAgentDoc)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	home := t.TempDir()

	if _, err := Install(home, Claude, sampleSkill, sampleAgentDoc); err != nil {
		t.Fatalf("first install: %v", err)
	}
	res, err := Install(home, Claude, sampleSkill, sampleAgentDoc)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if res.Changed {
		t.Error("re-installing identical content should report Changed=false")
	}
}

func TestInstallOverwritesWhenContentChanges(t *testing.T) {
	home := t.TempDir()

	first, err := Install(home, Claude, sampleSkill, sampleAgentDoc)
	if err != nil {
		t.Fatalf("first install: %v", err)
	}

	changedSkill := strings.Replace(sampleSkill, "A test skill.", "An updated skill.", 1)
	second, err := Install(home, Claude, changedSkill, sampleAgentDoc)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if !second.Changed {
		t.Error("changed content should report Changed=true (version-stamped overwrite)")
	}
	if second.Version == first.Version {
		t.Error("changed content should produce a different version")
	}

	got, err := os.ReadFile(filepath.Join(second.Dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read updated SKILL.md: %v", err)
	}
	if !strings.Contains(string(got), "An updated skill.") {
		t.Errorf("updated skill not written:\n%s", string(got))
	}
}

func TestInstallReinstallsWhenReferenceDrifts(t *testing.T) {
	home := t.TempDir()

	res, err := Install(home, Claude, sampleSkill, sampleAgentDoc)
	if err != nil {
		t.Fatalf("first install: %v", err)
	}
	// Corrupt the reference doc without touching the version stamp: the next
	// install must repair it.
	refPath := filepath.Join(res.Dir, "references", "AGENT.md")
	if err := os.WriteFile(refPath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := Install(home, Claude, sampleSkill, sampleAgentDoc)
	if err != nil {
		t.Fatalf("repair install: %v", err)
	}
	if !again.Changed {
		t.Error("a drifted reference doc should trigger a rewrite")
	}
	refData, _ := os.ReadFile(refPath)
	if string(refData) != sampleAgentDoc {
		t.Errorf("reference doc not repaired: %q", string(refData))
	}
}

func TestInstallDir(t *testing.T) {
	home := "/home/u"
	if got := InstallDir(home, Claude); got != filepath.Join(home, ".claude", "skills", "slis") {
		t.Errorf("claude dir = %q", got)
	}
	if got := InstallDir(home, Codex); got != filepath.Join(home, ".agents", "skills", "slis") {
		t.Errorf("codex dir = %q", got)
	}
}

func TestTargetHarnesses(t *testing.T) {
	cases := map[string][]Harness{
		"":       AllHarnesses,
		"both":   AllHarnesses,
		"claude": {Claude},
		"codex":  {Codex},
	}
	for flag, want := range cases {
		got, err := TargetHarnesses(flag)
		if err != nil {
			t.Errorf("TargetHarnesses(%q): %v", flag, err)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("TargetHarnesses(%q) = %v, want %v", flag, got, want)
		}
	}
	if _, err := TargetHarnesses("bogus"); err == nil {
		t.Error("TargetHarnesses(bogus) should error")
	}
}
