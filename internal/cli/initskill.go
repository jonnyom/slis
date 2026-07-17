package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	slis "github.com/jonnyom/slis"
	"github.com/jonnyom/slis/internal/skill"
)

// installSkill installs the embedded slis agent skill (and its reference doc)
// for each harness, printing what changed. Shared by `slis init-skill` and
// `slis init`. Idempotent — only rewrites when the skill content has changed.
func installSkill(harnesses []skill.Harness) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	for _, h := range harnesses {
		res, err := skill.Install(home, h, slis.SkillMarkdown, slis.AgentDoc)
		if err != nil {
			return fmt.Errorf("install skill for %s: %w", h, err)
		}
		if res.Changed {
			fmt.Printf("installed slis skill for %s → %s (version %s)\n", h, res.Dir, res.Version)
		} else {
			fmt.Printf("slis skill for %s already up to date → %s\n", h, res.Dir)
		}
	}
	return nil
}

var initSkillCmd = &cobra.Command{
	Use:   "init-skill",
	Short: "Install the slis agent skill for Claude Code and/or Codex",
	Long: `Installs the embedded slis agent skill (skills/slis/SKILL.md) plus its
reference doc (docs/AGENT.md → references/AGENT.md) into each harness's skills
directory:

  claude → ~/.claude/skills/slis/
  codex  → ~/.agents/skills/slis/   (the Agent Skills open standard)

Idempotent — re-running only rewrites when the skill content has changed.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		flag, _ := cmd.Flags().GetString("harness")
		harnesses, err := skill.TargetHarnesses(flag)
		if err != nil {
			return err
		}
		return installSkill(harnesses)
	},
}

func init() {
	initSkillCmd.Flags().String("harness", "both", "Which harness to install for: claude, codex, or both")
	rootCmd.AddCommand(initSkillCmd)
}
