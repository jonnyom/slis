package zmxctl

import "testing"

func TestParseListReturnsSessionMetadataAndLabels(t *testing.T) {
	output := "  name=slis-spike\tpid=79804\tclients=1\tcreated=1786461377\tstart_dir=/private/tmp\tcmd=/bin/sh -lc 'exec sleep 120'\tapp=slis\tgroup=checkout\ttab=agent\n"

	sessions, err := ParseList(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("session count = %d, want 1", len(sessions))
	}
	session := sessions[0]
	if session.Name != "slis-spike" || session.PID != 79804 || session.Clients != 1 {
		t.Fatalf("identity = %#v", session)
	}
	if session.Created != 1786461377 || session.StartDir != "/private/tmp" {
		t.Fatalf("lifecycle = %#v", session)
	}
	if session.Command != "/bin/sh -lc 'exec sleep 120'" {
		t.Fatalf("command = %q", session.Command)
	}
	if session.Labels["app"] != "slis" || session.Labels["group"] != "checkout" || session.Labels["tab"] != "agent" {
		t.Fatalf("labels = %#v", session.Labels)
	}
}
