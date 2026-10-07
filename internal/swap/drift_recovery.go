package swap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func ArchiveDriftedActivation(journalPath string) (string, error) {
	journal, err := Load(journalPath)
	if err != nil || journal == nil {
		return "", err
	}
	drifted := false
	for _, repo := range journal.Repos {
		err := validatePrimaryMirror(repo)
		if errors.Is(err, ErrUnknownPrimaryChanges) {
			drifted = true
		} else if err != nil {
			return "", err
		}
	}
	if !drifted {
		return "", nil
	}
	recoveryDirectory, err := os.MkdirTemp(filepath.Dir(journalPath), "activation-recovery-")
	if err != nil {
		return "", err
	}
	recoveryPath := filepath.Join(recoveryDirectory, "active.json")
	if err := os.Rename(journalPath, recoveryPath); err != nil {
		return "", errors.Join(err, os.Remove(recoveryDirectory))
	}
	return recoveryPath, nil
}

func liveBranchForActivation(slice, journalPath string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(journalPath), "activation-recovery-*", "active.json"))
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		recovered, err := Load(path)
		if err != nil {
			return "", err
		}
		if recovered != nil && recovered.Slice == slice {
			return fmt.Sprintf("%s-recovered-%d", LiveBranchName(slice), time.Now().UnixNano()), nil
		}
	}
	return LiveBranchName(slice), nil
}
