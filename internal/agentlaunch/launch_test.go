package agentlaunch

import (
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/model"
)

func TestLineUsesHarnessForShellQuotedClaudeBinary(t *testing.T) {
	line := Line("'/tmp/Claude Code/claude' '--model' 'fast model'", model.Slice{Name: "feature"}, "/work", "claude")
	if !strings.Contains(line, "--append-system-prompt") {
		t.Fatalf("line = %q", line)
	}
}
