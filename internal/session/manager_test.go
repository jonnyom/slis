package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/proc"
	"github.com/jonnyom/slis/internal/zmxctl"
)

func TestShouldStartLoginShellMigratesOnlyIdleBootstrapBash(t *testing.T) {
	tests := []struct {
		name       string
		loginShell string
		processes  []proc.ProcInfo
		want       bool
	}{
		{name: "idle bash to zsh", loginShell: "/bin/zsh", processes: []proc.ProcInfo{{PID: 10, Cmd: "-bash"}}, want: true},
		{name: "bash child exists", loginShell: "/bin/zsh", processes: []proc.ProcInfo{{PID: 10, Cmd: "-bash"}, {PID: 11, PPID: 10, Cmd: "sleep 30"}}},
		{name: "already zsh", loginShell: "/bin/zsh", processes: []proc.ProcInfo{{PID: 10, Cmd: "/bin/zsh -l"}}},
		{name: "user chose bash", loginShell: "/bin/bash", processes: []proc.ProcInfo{{PID: 10, Cmd: "-bash"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldStartLoginShell(test.loginShell, test.processes); got != test.want {
				t.Fatalf("shouldStartLoginShell() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestManagerActivatePersistsExplicitTab(t *testing.T) {
	store := OpenStore(t.TempDir())
	group := Group{
		ID:          "feature",
		ActiveTabID: "root",
		Tabs: []Tab{
			{ID: "root"},
			{ID: "repo-api"},
		},
	}
	if err := store.Save(group); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(store, nil)
	if err := manager.Activate("feature", "repo-api"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Get("feature")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ActiveTabID != "repo-api" {
		t.Fatalf("active tab = %q, want repo-api", reloaded.ActiveTabID)
	}
}

func TestManagerKillPreservesStateWhenTerminalKillFails(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "zmx")
	script := "#!/bin/sh\nif [ \"$2\" = \"bad-terminal\" ]; then exit 1; fi\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	store := OpenStore(directory)
	group := Group{
		ID: "feature",
		Tabs: []Tab{
			{ID: "root", PersistenceName: "good-terminal"},
			{ID: "repo-api", PersistenceName: "bad-terminal"},
		},
	}
	if err := store.Save(group); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, zmxctl.New(binary, "/tmp/slis-session-test"))

	if err := manager.Kill(context.Background(), group.ID); err == nil {
		t.Fatal("kill returned nil error")
	}
	if _, err := store.Get(group.ID); err != nil {
		t.Fatalf("group state removed after failed kill: %v", err)
	}
}

func TestManagerKillTabStopsOnlySelectedTerminalAndKeepsGroup(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "zmx")
	killedPath := filepath.Join(directory, "killed")
	script := "#!/bin/sh\nprintf '%s' \"$2\" > \"$ZMX_TEST_KILLED\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZMX_TEST_KILLED", killedPath)
	store := OpenStore(directory)
	group := Group{
		ID:          "feature",
		ActiveTabID: "agent-2",
		Tabs: []Tab{
			{ID: "root", PersistenceName: "terminal-root"},
			{ID: "agent", PersistenceName: "terminal-agent"},
			{ID: "agent-2", PersistenceName: "terminal-agent-2"},
		},
		Deliveries: map[string]Delivery{
			"keep":   {ID: "keep", TabID: "agent", Status: DeliverySent},
			"remove": {ID: "remove", TabID: "agent-2", Status: DeliverySent},
		},
	}
	if err := store.Save(group); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, zmxctl.New(binary, "/tmp/slis-session-test"))

	if err := manager.KillTab(context.Background(), group.ID, "agent-2"); err != nil {
		t.Fatal(err)
	}
	killed, err := os.ReadFile(killedPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(killed), "terminal-agent-2"; got != want {
		t.Fatalf("killed terminal = %q, want %q", got, want)
	}
	reloaded, err := store.Get(group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Tabs) != 2 || reloaded.Tabs[0].ID != "root" || reloaded.Tabs[1].ID != "agent" {
		t.Fatalf("tabs = %#v", reloaded.Tabs)
	}
	if reloaded.ActiveTabID != "agent" {
		t.Fatalf("active tab = %q, want agent", reloaded.ActiveTabID)
	}
	if _, found := reloaded.Deliveries["remove"]; found {
		t.Fatal("closed tab delivery remains")
	}
	if _, found := reloaded.Deliveries["keep"]; !found {
		t.Fatal("other tab delivery was removed")
	}
}

func TestManagerSendOnceSerializesConcurrentDelivery(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "zmx")
	inputPath := filepath.Join(directory, "input")
	script := "#!/bin/sh\nprintf '%s' \"$3\" >> \"$ZMX_TEST_INPUT\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZMX_TEST_INPUT", inputPath)
	store := OpenStore(directory)
	group := Group{ID: "feature", Tabs: []Tab{{ID: "agent", PersistenceName: "terminal-agent"}}}
	if err := store.Save(group); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, zmxctl.New(binary, "/tmp/slis-session-test"))

	start := make(chan struct{})
	errorsByCall := make(chan error, 2)
	var calls sync.WaitGroup
	for range 2 {
		calls.Add(1)
		go func() {
			defer calls.Done()
			<-start
			errorsByCall <- manager.SendOnce(context.Background(), "feature", "agent", "review-1:request-1", []byte("prompt\r"))
		}()
	}
	close(start)
	calls.Wait()
	close(errorsByCall)
	for err := range errorsByCall {
		if err != nil {
			t.Fatal(err)
		}
	}
	contents, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(contents), "prompt\r"; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
}

func TestManagerSendOnceRefusesIndeterminatePreparedDelivery(t *testing.T) {
	directory := t.TempDir()
	store := OpenStore(directory)
	group := Group{
		ID:   "feature",
		Tabs: []Tab{{ID: "agent", PersistenceName: "terminal-agent"}},
		Deliveries: map[string]Delivery{
			"review-1:request-1": {ID: "review-1:request-1", TabID: "agent", Status: DeliveryPrepared},
		},
	}
	if err := store.Save(group); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, zmxctl.New("unused", "/tmp/slis-session-test"))

	err := manager.SendOnce(context.Background(), "feature", "agent", "review-1:request-1", []byte("prompt\r"))
	if !errors.Is(err, ErrDeliveryIndeterminate) {
		t.Fatalf("error = %v, want ErrDeliveryIndeterminate", err)
	}
}

func TestManagerEnsurePreservesTabAddedDuringReconciliation(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "zmx")
	signalPath := filepath.Join(directory, "listed")
	releasePath := filepath.Join(directory, "release")
	script := "#!/bin/sh\nif [ \"$1\" = \"list\" ]; then touch \"$ZMX_TEST_SIGNAL\"; while [ ! -f \"$ZMX_TEST_RELEASE\" ]; do sleep 0.01; done; fi\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZMX_TEST_SIGNAL", signalPath)
	t.Setenv("ZMX_TEST_RELEASE", releasePath)
	store := OpenStore(directory)
	if err := store.Save(Group{ID: "feature", ActiveTabID: "root", Tabs: []Tab{{ID: "root", Kind: TabKindRoot, CWD: directory, PersistenceName: "root-terminal"}}}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, zmxctl.New(binary, "/tmp/slis-session-test"))
	result := make(chan error, 1)
	go func() {
		_, err := manager.Ensure(context.Background(), "feature", []model.SliceMember{{Repo: "api", WorktreePath: directory}}, LayoutOptions{Root: directory})
		result <- err
	}()
	deadline := time.After(time.Second)
	for {
		if _, err := os.Stat(signalPath); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("session list did not start")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := store.Update("feature", func(group *Group) error {
		group.Tabs = append(group.Tabs, Tab{ID: "review-42", Kind: TabKindReview, CWD: directory, PersistenceName: "review-terminal"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(releasePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	group, err := store.Get("feature")
	if err != nil {
		t.Fatal(err)
	}
	if !groupHasTab(group, "review-42") {
		t.Fatal("review tab was lost during reconciliation")
	}
}
