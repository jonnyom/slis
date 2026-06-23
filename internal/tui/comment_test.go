package tui

import "testing"

func TestCleanCommentBody(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "looks good to me", "looks good to me"},
		{"strips html comment", "text <!-- linear-linkback --> more", "text more"},
		{"strips html tags", `see <a href="x"><img src="y" alt="Graphite"></a> ok`, "see ok"},
		{"collapses whitespace/newlines", "a\n\n  b\tc", "a b c"},
		{"keeps markdown link text, drops url", "review [#8062](https://app.graphite.dev/x) now", "review #8062 now"},
		{"drops markdown image", "before ![alt](http://img.png) after", "before after"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cleanCommentBody(c.in); got != c.want {
				t.Errorf("cleanCommentBody(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
