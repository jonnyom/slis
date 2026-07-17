package tmuxctl

import "testing"

func TestParseClients(t *testing.T) {
	raw := "/dev/ttys001\tslis/alpha\t1700000100\n" +
		"/dev/ttys004\tother\t1700000200\n" +
		"\n" + // blank line skipped
		"/dev/ttys009\tslis/beta\t1700000050\n"

	clients := parseClients(raw)
	if len(clients) != 3 {
		t.Fatalf("parseClients returned %d clients, want 3: %+v", len(clients), clients)
	}

	want := []Client{
		{TTY: "/dev/ttys001", Session: "slis/alpha", LastActivity: 1700000100},
		{TTY: "/dev/ttys004", Session: "other", LastActivity: 1700000200},
		{TTY: "/dev/ttys009", Session: "slis/beta", LastActivity: 1700000050},
	}
	for i, w := range want {
		if clients[i] != w {
			t.Errorf("client[%d] = %+v, want %+v", i, clients[i], w)
		}
	}
}

func TestParseClientsSkipsMalformed(t *testing.T) {
	// Missing activity field, missing tty, and empty input must not panic.
	raw := "/dev/ttys001\tslis/alpha\n" + // only 2 fields → skipped
		"\tslis/beta\t123\n" // empty tty → skipped
	if got := parseClients(raw); len(got) != 0 {
		t.Errorf("parseClients(malformed) = %+v, want empty", got)
	}
	if got := parseClients(""); len(got) != 0 {
		t.Errorf("parseClients(empty) = %+v, want empty", got)
	}
}

func TestMostRecentClient(t *testing.T) {
	if _, ok := MostRecentClient(nil); ok {
		t.Error("MostRecentClient(nil) ok=true, want false")
	}

	clients := []Client{
		{TTY: "/dev/ttys001", Session: "a", LastActivity: 100},
		{TTY: "/dev/ttys002", Session: "b", LastActivity: 300},
		{TTY: "/dev/ttys003", Session: "c", LastActivity: 200},
	}
	got, ok := MostRecentClient(clients)
	if !ok {
		t.Fatal("MostRecentClient ok=false, want true")
	}
	if got.TTY != "/dev/ttys002" {
		t.Errorf("MostRecentClient = %q, want /dev/ttys002 (highest activity)", got.TTY)
	}
}

func TestSwitchClientArgv(t *testing.T) {
	name, args := SwitchClientArgv("/dev/ttys007", "my.slice")
	if name != "tmux" {
		t.Errorf("binary = %q, want tmux", name)
	}
	want := []string{"switch-client", "-c", "/dev/ttys007", "-t", SessionName("my.slice")}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}
