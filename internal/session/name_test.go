package session

import (
	"regexp"
	"testing"
)

func TestPersistenceNameIsStableSafeAndBounded(t *testing.T) {
	first := PersistenceName("Feature/ABC with spaces", "repo:api")
	second := PersistenceName("Feature/ABC with spaces", "repo:api")
	other := PersistenceName("Feature/ABC with spaces", "repo:web")

	if first != second {
		t.Fatalf("name changed: %q then %q", first, second)
	}
	if first == other {
		t.Fatalf("different tabs share name %q", first)
	}
	if len(first) > 48 {
		t.Fatalf("name length = %d, want at most 48", len(first))
	}
	if !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(first) {
		t.Fatalf("unsafe name %q", first)
	}
}
