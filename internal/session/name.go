package session

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func PersistenceName(groupID, tabID string) string {
	identity := groupID + "\x00" + tabID
	digest := sha256.Sum256([]byte(identity))
	slug := safeNamePart(groupID + "-" + tabID)
	if len(slug) > 25 {
		slug = strings.Trim(slug[:25], "-")
	}
	if slug == "" {
		slug = "terminal"
	}
	return "slis-" + slug + "-" + hex.EncodeToString(digest[:6])
}

func safeNamePart(value string) string {
	var result strings.Builder
	separator := false
	for _, character := range strings.ToLower(value) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			result.WriteRune(character)
			separator = false
			continue
		}
		if result.Len() > 0 && !separator {
			result.WriteByte('-')
			separator = true
		}
	}
	return strings.Trim(result.String(), "-")
}
