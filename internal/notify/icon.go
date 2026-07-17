package notify

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/jonnyom/slis/internal/config"
)

//go:embed slis.png
var iconPNG []byte

// iconPath extracts the embedded slis icon into the XDG state directory
// (<state>/slis.png) and returns its path. It writes the file only when it is
// missing or a different size, so repeated calls are cheap. An extraction
// failure returns a non-empty error and an empty path; callers should proceed
// without an icon rather than dropping the notification.
func iconPath() (string, error) {
	dest := filepath.Join(config.StatePaths().StateDir, "slis.png")
	if info, err := os.Stat(dest); err == nil && info.Size() == int64(len(iconPNG)) {
		return dest, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, iconPNG, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}
