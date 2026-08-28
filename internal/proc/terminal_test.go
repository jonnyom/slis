package proc

import "testing"

func TestParseTerminalProcessGroupsDetectsForegroundCommand(t *testing.T) {
	busy, err := parseTerminalProcessGroups(" 15140 15403\n")
	if err != nil {
		t.Fatal(err)
	}
	if !busy {
		t.Fatal("different foreground process group reported idle")
	}
}

func TestParseTerminalProcessGroupsTreatsPromptHelpersAsIdle(t *testing.T) {
	busy, err := parseTerminalProcessGroups(" 15140 15140\n")
	if err != nil {
		t.Fatal(err)
	}
	if busy {
		t.Fatal("shell foreground process group reported busy")
	}
}

func TestParseTerminalProcessGroupsRejectsUnexpectedOutput(t *testing.T) {
	if _, err := parseTerminalProcessGroups("15140\n"); err == nil {
		t.Fatal("unexpected ps output accepted")
	}
}

func TestParseTerminalProcessGroupsByPID(t *testing.T) {
	busy, err := parseTerminalProcessGroupsByPID(" 101 101 202\n 303 303 303\n")
	if err != nil {
		t.Fatalf("parseTerminalProcessGroupsByPID: %v", err)
	}
	if !busy[101] || busy[303] {
		t.Fatalf("busy = %#v", busy)
	}
}
