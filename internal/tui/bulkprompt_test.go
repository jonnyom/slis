package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
)

func nFakeSlices(n int) []model.Slice {
	s := make([]model.Slice, n)
	for i := range s {
		s[i] = model.Slice{Name: fmt.Sprintf("slice-%d", i)}
	}
	return s
}

func newBulkModel(t *testing.T) Model {
	t.Helper()
	m := New(config.Workspace{})
	m.loading = false
	return m
}

// TestBulkPromptAboveThreshold: a cold load of >bulkLoadThreshold slices must
// raise the prompt and NOT fan out the whole-workspace card/PR batch (only the
// focused slice loads, via the preview).
func TestBulkPromptAboveThreshold(t *testing.T) {
	m := newBulkModel(t)
	next, _ := m.Update(slicesLoadedMsg{slices: nFakeSlices(bulkLoadThreshold + 1)})
	m = next.(Model)

	if !m.bulkPrompt {
		t.Fatal("expected bulkPrompt to be set above threshold")
	}
	if len(m.cardLoading) != 1 {
		t.Fatalf("expected only the focused card to load (1), got %d — batch fanned out", len(m.cardLoading))
	}
}

// TestNoBulkPromptBelowThreshold: small workspaces load eagerly as before.
func TestNoBulkPromptBelowThreshold(t *testing.T) {
	m := newBulkModel(t)
	n := bulkLoadThreshold - 5
	next, _ := m.Update(slicesLoadedMsg{slices: nFakeSlices(n)})
	m = next.(Model)

	if m.bulkPrompt {
		t.Fatal("did not expect bulkPrompt below threshold")
	}
	if len(m.cardLoading) != n {
		t.Fatalf("expected all %d cards to batch-load, got %d", n, len(m.cardLoading))
	}
}

// TestBulkPromptLoadAll: pressing [y] dismisses the prompt and loads the whole
// workspace.
func TestBulkPromptLoadAll(t *testing.T) {
	m := newBulkModel(t)
	n := bulkLoadThreshold + 1
	next, _ := m.Update(slicesLoadedMsg{slices: nFakeSlices(n)})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(Model)

	if m.bulkPrompt {
		t.Fatal("[y] should dismiss the prompt")
	}
	if m.lazyCards {
		t.Fatal("[y] should not enter lazy mode")
	}
	if len(m.cardLoading) != n {
		t.Fatalf("[y] should batch-load all %d cards, got %d", n, len(m.cardLoading))
	}
}

// TestBulkPromptLazy: pressing [n] dismisses the prompt, enters lazy mode, and a
// subsequent reload neither re-prompts nor fans out the batch.
func TestBulkPromptLazy(t *testing.T) {
	m := newBulkModel(t)
	n := bulkLoadThreshold + 1
	next, _ := m.Update(slicesLoadedMsg{slices: nFakeSlices(n)})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(Model)

	if m.bulkPrompt {
		t.Fatal("[n] should dismiss the prompt")
	}
	if !m.lazyCards {
		t.Fatal("[n] should enter lazy mode")
	}

	// Reload: still large, but the user already chose lazy — no re-prompt, no batch.
	next, _ = m.Update(slicesLoadedMsg{slices: nFakeSlices(n)})
	m = next.(Model)
	if m.bulkPrompt {
		t.Fatal("lazy mode should not re-prompt on reload")
	}
	if len(m.cardLoading) > 1 {
		t.Fatalf("lazy mode should not batch-load on reload, got %d loading", len(m.cardLoading))
	}
}
