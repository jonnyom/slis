package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderCIRerun(t *testing.T) {
	rows := []ciRerunRow{
		{repo: "web", n: 2},
		{repo: "api", n: 0},
		{repo: "ops", err: errors.New("gh not found on PATH")},
	}
	got := renderCIRerun(rows)

	for _, want := range []string{
		"web: re-triggered 2 run(s)",
		"api: no failing runs to re-trigger",
		"ops: error: gh not found on PATH",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderCIRerun output missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestRenderCIRerunEmpty(t *testing.T) {
	got := renderCIRerun(nil)
	if !strings.Contains(got, "no PRs with failing CI") {
		t.Errorf("empty renderCIRerun = %q, want a no-PRs message", got)
	}
}

func TestCIRerunAnySucceeded(t *testing.T) {
	cases := []struct {
		name string
		rows []ciRerunRow
		want bool
	}{
		{"one re-triggered", []ciRerunRow{{repo: "web", n: 1}}, true},
		{"only zero-count", []ciRerunRow{{repo: "web", n: 0}}, false},
		{"only errors", []ciRerunRow{{repo: "web", err: errors.New("boom")}}, false},
		{"error plus success", []ciRerunRow{{repo: "web", err: errors.New("boom")}, {repo: "api", n: 3}}, true},
		{"empty", nil, false},
	}
	for _, c := range cases {
		if got := ciRerunAnySucceeded(c.rows); got != c.want {
			t.Errorf("%s: ciRerunAnySucceeded = %v, want %v", c.name, got, c.want)
		}
	}
}
