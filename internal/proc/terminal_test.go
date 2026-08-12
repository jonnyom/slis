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
