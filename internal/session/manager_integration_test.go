package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/zmxctl"
)

func TestManagerEnsureReconcilesNewTabsWithRealZmx(t *testing.T) {
	binary := os.Getenv("SLIS_ZMX_BINARY")
	if binary == "" {
		t.Skip("SLIS_ZMX_BINARY is not set")
	}

	root := t.TempDir()
	members := []model.SliceMember{
		{Repo: "api", WorktreePath: filepath.Join(root, "feature", "api")},
		{Repo: "web", WorktreePath: filepath.Join(root, "feature", "web")},
	}
	for _, member := range members {
		if err := os.MkdirAll(member.WorktreePath, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	runtimeDirectory, err := os.MkdirTemp("/tmp", "slis-zmx-manager-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDirectory) })
	client := zmxctl.New(binary, runtimeDirectory)
	manager := NewManager(OpenStore(t.TempDir()), client)
	ctx := context.Background()
	group, err := manager.Ensure(ctx, "feature", members, LayoutOptions{Root: root, Layout: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Tabs) != 1 {
		t.Fatalf("initial tabs = %#v, want one root tab", group.Tabs)
	}
	for _, tab := range group.Tabs {
		t.Cleanup(func() { _ = client.Kill(ctx, tab.PersistenceName) })
	}
	if err := manager.Send(ctx, "feature", "root", []byte("if [[ -n \"$ZSH_VERSION\" ]]; then printf '%s%s\\n' SLIS_ZSH _ACTIVE; fi\r")); err != nil {
		t.Fatal(err)
	}
	loginShellDeadline := time.Now().Add(3 * time.Second)
	for {
		history, historyErr := manager.History(ctx, "feature", "root", false)
		if historyErr != nil {
			t.Fatal(historyErr)
		}
		if strings.Contains(history, "SLIS_ZSH_ACTIVE") {
			break
		}
		if time.Now().After(loginShellDeadline) {
			t.Fatalf("login zsh marker missing from history: %q", history)
		}
		time.Sleep(10 * time.Millisecond)
	}

	group, err = manager.Ensure(ctx, "feature", members, LayoutOptions{Root: root, Layout: "both"})
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Tabs) != 3 {
		t.Fatalf("reconciled tabs = %#v, want root and two repo tabs", group.Tabs)
	}
	for _, tab := range group.Tabs[1:] {
		t.Cleanup(func() { _ = client.Kill(ctx, tab.PersistenceName) })
	}

	sessions, err := client.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tab := range group.Tabs {
		if !containsPersistenceSession(sessions, tab.PersistenceName) {
			t.Fatalf("terminal %q missing from %#v", tab.PersistenceName, sessions)
		}
	}
}

func containsPersistenceSession(sessions []zmxctl.Session, name string) bool {
	for _, session := range sessions {
		if session.Name == name {
			return true
		}
	}
	return false
}

func TestManagerRoutesInputAndHistoryByExplicitTabWithRealZmx(t *testing.T) {
	binary := os.Getenv("SLIS_ZMX_BINARY")
	if binary == "" {
		t.Skip("SLIS_ZMX_BINARY is not set")
	}

	root := t.TempDir()
	members := []model.SliceMember{
		{Repo: "api", WorktreePath: filepath.Join(root, "feature", "api")},
		{Repo: "web", WorktreePath: filepath.Join(root, "feature", "web")},
	}
	for _, member := range members {
		if err := os.MkdirAll(member.WorktreePath, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runtimeDirectory, err := os.MkdirTemp("/tmp", "slis-zmx-routing-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDirectory) })
	client := zmxctl.New(binary, runtimeDirectory)
	manager := NewManager(OpenStore(t.TempDir()), client)
	ctx := context.Background()
	group, err := manager.Ensure(ctx, "feature", members, LayoutOptions{Root: root, Layout: "both"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tab := range group.Tabs {
		tab := tab
		t.Cleanup(func() { _ = client.Kill(ctx, tab.PersistenceName) })
	}

	if err := manager.Send(ctx, "feature", "repo-api", []byte("printf 'SLIS_EXPLICIT_TAB\\n'\r")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		history, historyErr := manager.History(ctx, "feature", "repo-api", false)
		if historyErr != nil {
			t.Fatal(historyErr)
		}
		if strings.Contains(history, "SLIS_EXPLICIT_TAB") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("explicit tab history missing marker: %q", history)
		}
		time.Sleep(10 * time.Millisecond)
	}
	rootHistory, err := manager.History(ctx, "feature", "root", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rootHistory, "SLIS_EXPLICIT_TAB") {
		t.Fatalf("root history received repo input: %q", rootHistory)
	}
	capture, err := manager.CaptureGroup(ctx, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capture, "── api ──") || !strings.Contains(capture, "SLIS_EXPLICIT_TAB") {
		t.Fatalf("group capture missing tab heading or output: %q", capture)
	}
}

func TestManagerStartsLongCommandWithoutTruncationWithRealZmx(t *testing.T) {
	binary := os.Getenv("SLIS_ZMX_BINARY")
	if binary == "" {
		t.Skip("SLIS_ZMX_BINARY is not set")
	}

	root := t.TempDir()
	worktreePath := filepath.Join(root, "feature", "api")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatal(err)
	}
	runtimeDirectory, err := os.MkdirTemp("/tmp", "slis-zmx-long-input-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDirectory) })
	client := zmxctl.New(binary, runtimeDirectory)
	manager := NewManager(OpenStore(t.TempDir()), client)
	ctx := context.Background()
	group, err := manager.Ensure(ctx, "feature", []model.SliceMember{{Repo: "api", WorktreePath: worktreePath}}, LayoutOptions{Root: root, Layout: "root"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tab := range group.Tabs {
		tab := tab
		t.Cleanup(func() { _ = client.Kill(ctx, tab.PersistenceName) })
	}

	payload := strings.Repeat("x", 16*1024) + "SLIS_LONG_INPUT_END"
	resultPath := filepath.Join(root, "long-input-result")
	command := "printf '%s' '" + payload + "' > '" + resultPath + "'; sleep 1"
	if err := manager.StartExistingCommand(ctx, "feature", "root", command); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		result, readErr := os.ReadFile(resultPath)
		if readErr == nil {
			if string(result) != payload {
				t.Fatalf("long input result length = %d, want %d", len(result), len(payload))
			}
			break
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatal(readErr)
		}
		if time.Now().After(deadline) {
			history, historyErr := manager.History(ctx, "feature", "root", false)
			if historyErr != nil {
				t.Fatal(historyErr)
			}
			t.Fatalf("long input did not execute; history length = %d", len(history))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestManagerEnsureTabAddsAgentTerminalWithoutReplacingLayoutTabs(t *testing.T) {
	binary := os.Getenv("SLIS_ZMX_BINARY")
	if binary == "" {
		t.Skip("SLIS_ZMX_BINARY is not set")
	}

	worktree := t.TempDir()
	members := []model.SliceMember{{Repo: "api", WorktreePath: worktree}}
	runtimeDirectory, err := os.MkdirTemp("/tmp", "slis-zmx-add-tab-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDirectory) })
	client := zmxctl.New(binary, runtimeDirectory)
	manager := NewManager(OpenStore(t.TempDir()), client)
	ctx := context.Background()
	group, err := manager.Ensure(ctx, "feature", members, LayoutOptions{Root: worktree, Layout: "root"})
	if err != nil {
		t.Fatal(err)
	}
	group, err = manager.EnsureTab(ctx, "feature", TabSpec{ID: "agent", Kind: TabKindAgent, Title: "agent", CWD: worktree})
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Tabs) != 2 || group.Tabs[0].ID != "root" || group.Tabs[1].ID != "agent" {
		t.Fatalf("tabs = %#v, want root then agent", group.Tabs)
	}
	group, err = manager.Ensure(ctx, "feature", members, LayoutOptions{Root: worktree, Layout: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Tabs) != 2 || group.Tabs[1].ID != "agent" {
		t.Fatalf("reconciled tabs = %#v, want dynamic agent tab preserved", group.Tabs)
	}
	for _, tab := range group.Tabs {
		tab := tab
		t.Cleanup(func() { _ = client.Kill(ctx, tab.PersistenceName) })
	}
}

func TestManagerBusyUsesTerminalForegroundProcessGroupWithRealZmx(t *testing.T) {
	binary := os.Getenv("SLIS_ZMX_BINARY")
	if binary == "" {
		t.Skip("SLIS_ZMX_BINARY is not set")
	}

	worktree := t.TempDir()
	runtimeDirectory, err := os.MkdirTemp("/tmp", "slis-zmx-busy-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDirectory) })
	client := zmxctl.New(binary, runtimeDirectory)
	manager := NewManager(OpenStore(t.TempDir()), client)
	ctx := context.Background()
	group, err := manager.Ensure(ctx, "feature", []model.SliceMember{{Repo: "api", WorktreePath: worktree}}, LayoutOptions{Root: worktree, Layout: "root"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Kill(ctx, group.Tabs[0].PersistenceName) })
	busy, err := manager.Busy(ctx, "feature", "root")
	if err != nil {
		t.Fatal(err)
	}
	if busy {
		t.Fatal("fresh shell reported busy")
	}
	if err := manager.Send(ctx, "feature", "root", []byte("sleep 3\r")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		busy, err = manager.Busy(ctx, "feature", "root")
		if err != nil {
			t.Fatal(err)
		}
		if busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("running child process never reported busy")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestManagerStartCommandRejectsBusyTerminalWithRealZmx(t *testing.T) {
	binary := os.Getenv("SLIS_ZMX_BINARY")
	if binary == "" {
		t.Skip("SLIS_ZMX_BINARY is not set")
	}

	worktree := t.TempDir()
	runtimeDirectory, err := os.MkdirTemp("/tmp", "slis-zmx-command-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDirectory) })
	client := zmxctl.New(binary, runtimeDirectory)
	manager := NewManager(OpenStore(t.TempDir()), client)
	ctx := context.Background()
	group, err := manager.Ensure(ctx, "feature", []model.SliceMember{{Repo: "api", WorktreePath: worktree}}, LayoutOptions{Root: worktree, Layout: "root"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Kill(ctx, group.Tabs[0].PersistenceName) })
	if err := manager.StartCommand(ctx, "feature", TabSpec{ID: "review-1", Kind: TabKindReview, Title: "review", CWD: worktree}, "sleep 3"); err != nil {
		t.Fatal(err)
	}
	err = manager.StartCommand(ctx, "feature", TabSpec{ID: "review-1", Kind: TabKindReview, Title: "review", CWD: worktree}, "printf duplicate")
	if !errors.Is(err, ErrTerminalBusy) {
		t.Fatalf("second command error = %v, want ErrTerminalBusy", err)
	}
	reviewTab, err := manager.tab("feature", "review-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Kill(ctx, reviewTab.PersistenceName) })
}
