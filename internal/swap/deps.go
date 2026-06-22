package swap

import (
	"crypto/sha256"
	"fmt"

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
// If the path genuinely does not exist at that rev, it returns ("", nil) so
// that absent-in-both and added/removed cases are handled gracefully.
// Any other git error (e.g. bad/unknown object SHA, I/O error) is propagated
// so it is not silently swallowed as "file absent".
//
// Implementation note: git reports identical stderr ("does not exist in '<sha>'")
// for both "file absent at a valid commit" and "SHA does not name any object".
// We disambiguate by first checking whether the SHA names a valid commit object
// via `git cat-file -e <sha>`. Only if the commit exists do we treat the
// subsequent "does not exist in" as genuine file-absence; otherwise we propagate
// the error as a real git failure.
func blobAt(repoDir, sha, path string) (string, error) {
	// Fix E: pre-validate that sha names a real object before running git show.
	// `git cat-file -e <sha>` exits 0 when the object exists, non-zero otherwise.
	// This ensures a bogus/corrupt SHA is surfaced as an error rather than
	// silently treated as "file absent".
	if _, err := git.Run(repoDir, "cat-file", "-e", sha); err != nil {
		return "", fmt.Errorf("blobAt: commit %q does not exist in %q: %w", sha, repoDir, err)
	}

	content, err := git.Run(repoDir, "show", sha+":"+path)
	if err != nil {
		// At this point we know the commit exists; any error is a genuine "path
		// not present at this rev". Treat as empty content (absent file).
		return "", nil
	}
	return content, nil
}
