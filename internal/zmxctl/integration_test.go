package zmxctl

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClientLifecycleWithRealZmx(t *testing.T) {
	binary := os.Getenv("SLIS_ZMX_BINARY")
	if binary == "" {
		t.Skip("SLIS_ZMX_BINARY is not set")
	}

	client := New(binary, t.TempDir())
	ctx := context.Background()
	name := "lifecycle"
	if err := client.Ensure(ctx, name, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Kill(ctx, name) })

	sessions, err := client.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSession(sessions, name) {
		t.Fatalf("session %q missing from %#v", name, sessions)
	}

	input := []byte("printf 'SLIS_ZMX_FIRST\\nSLIS_ZMX_SECOND\\n'\r")
	if err := client.Send(ctx, name, input); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		history, historyErr := client.History(ctx, name, false)
		if historyErr != nil {
			t.Fatal(historyErr)
		}
		if strings.Contains(history, "SLIS_ZMX_FIRST\nSLIS_ZMX_SECOND\n") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("history did not contain expected lines: %q", history)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := client.Kill(ctx, name); err != nil {
		t.Fatal(err)
	}
	sessions, err = client.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if containsSession(sessions, name) {
		t.Fatalf("session %q remains after kill", name)
	}
}

func containsSession(sessions []Session, name string) bool {
	for _, session := range sessions {
		if session.Name == name {
			return true
		}
	}
	return false
}
