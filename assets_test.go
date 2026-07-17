package slis_test

import (
	"regexp"
	"testing"

	slis "github.com/jonnyom/slis"
	"github.com/jonnyom/slis/internal/skill"
)

// skillNameRe is the Agent Skills spec name pattern.
var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// TestEmbeddedSkillFrontmatter validates the embedded SKILL.md frontmatter
// against the Agent Skills spec: the name matches the directory (skills/slis)
// and the regex, and the description is present and within the 1024-char limit.
func TestEmbeddedSkillFrontmatter(t *testing.T) {
	fm, err := skill.ParseFrontmatter(slis.SkillMarkdown)
	if err != nil {
		t.Fatalf("parse embedded skill frontmatter: %v", err)
	}

	if fm.Name != "slis" {
		t.Errorf("skill name = %q, want %q (must match the skills/slis directory)", fm.Name, "slis")
	}
	if !skillNameRe.MatchString(fm.Name) {
		t.Errorf("skill name %q does not match %s", fm.Name, skillNameRe)
	}
	if fm.Description == "" {
		t.Error("skill description must be non-empty")
	}
	if len(fm.Description) > 1024 {
		t.Errorf("skill description is %d chars, exceeds the 1024-char limit", len(fm.Description))
	}
}

// TestEmbeddedAgentDocPresent guards against an empty embed.
func TestEmbeddedAgentDocPresent(t *testing.T) {
	if len(slis.AgentDoc) == 0 {
		t.Error("embedded AgentDoc is empty")
	}
}
