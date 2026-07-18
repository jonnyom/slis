package cli

import (
	"testing"

	"github.com/jonnyom/slis/internal/config"
)

func TestAgentDefaultCommandsPersistWorkspaceConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := config.WorkspacePath()
	ws := config.Workspace{
		Root: t.TempDir(),
		Repos: map[string]config.Repo{
			"web": {Primary: t.TempDir(), DefaultBranch: "main"},
		},
	}
	if err := config.SaveWorkspace(path, ws); err != nil {
		t.Fatal(err)
	}

	if err := agentSetDefaultCmd.RunE(agentSetDefaultCmd, []string{"Codex"}); err != nil {
		t.Fatal(err)
	}
	got, err := config.LoadWorkspace(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sessions.DefaultAgent != "Codex" {
		t.Fatalf("default agent = %q, want Codex", got.Sessions.DefaultAgent)
	}

	if err := agentClearDefaultCmd.RunE(agentClearDefaultCmd, nil); err != nil {
		t.Fatal(err)
	}
	got, err = config.LoadWorkspace(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sessions.DefaultAgent != "" {
		t.Fatalf("default agent = %q after clear, want empty", got.Sessions.DefaultAgent)
	}
}
