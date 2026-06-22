package swap

import (
	"crypto/sha256"

	"github.com/jonnyom/slis/internal/git"
)

// LockfilesChanged reports whether any of the given lockfile paths differ
// between fromSHA and toSHA in the repository at repoDir.
//
// Each lockfile is retrieved via `git show <sha>:<path>`. If a path is absent
// at a given rev, its content is treated as empty. The comparison is a sha256
// hash of the (trimmed) blob content on both sides; if any lockfile hash
// differs, the function returns (true, nil) immediately (short-circuit OR).
// If all lockfiles are identical (including both absent), it returns (false, nil).
func LockfilesChanged(repoDir, fromSHA, toSHA string, lockfiles []string) (bool, error) {
	for _, lf := range lockfiles {
		fromContent, err := blobAt(repoDir, fromSHA, lf)
		if err != nil {
			return false, err
		}
		toContent, err := blobAt(repoDir, toSHA, lf)
		if err != nil {
			return false, err
		}
		if sha256.Sum256([]byte(fromContent)) != sha256.Sum256([]byte(toContent)) {
			return true, nil
		}
	}
	return false, nil
}

// blobAt returns the content of path at sha in repoDir via `git show sha:path`.
// If the path does not exist at that rev, it returns ("", nil) rather than an
// error so that absent-in-both and added/removed cases are handled gracefully.
func blobAt(repoDir, sha, path string) (string, error) {
	content, err := git.Run(repoDir, "show", sha+":"+path)
	if err != nil {
		// "path not in rev" — treat as empty content.
		return "", nil
	}
	return content, nil
}
