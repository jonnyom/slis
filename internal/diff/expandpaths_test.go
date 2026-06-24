package diff_test

import (
	"reflect"
	"testing"

	"github.com/jonnyom/slis/internal/diff"
)

func TestExpandPaths(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"src/app.go", []string{"src/app.go"}},
		{"  spaced.go  ", []string{"spaced.go"}},
		{"foo.go => bar.go", []string{"foo.go", "bar.go"}},
		{"a/{old => new}/f.go", []string{"a/old/f.go", "a/new/f.go"}},
		{"{old => new}", []string{"old", "new"}},
		{"pkg/{a => b}.go", []string{"pkg/a.go", "pkg/b.go"}},
		{"a/{ => sub}/f.go", []string{"a/f.go", "a/sub/f.go"}},
		{"same.go => same.go", []string{"same.go"}},
	}
	for _, c := range cases {
		got := diff.ExpandPaths(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ExpandPaths(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
