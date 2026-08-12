package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const focusRequestFile = "focus-request.json"
const focusAckFile = "focus-ack.json"

type FocusRequest struct {
	ID      string `json:"id"`
	GroupID string `json:"group_id"`
	TabID   string `json:"tab_id"`
	TimeNS  int64  `json:"time_ns"`
}

func (request FocusRequest) Fresh(now time.Time) bool {
	created := time.Unix(0, request.TimeNS)
	return request.TimeNS != 0 && !created.After(now.Add(time.Second)) && now.Sub(created) <= 5*time.Second
}

type focusAck struct {
	ID string `json:"id"`
}

func WriteFocusRequest(stateDirectory string, request FocusRequest) error {
	return writeFocusJSON(stateDirectory, focusRequestFile, request)
}

func ReadFocusRequest(stateDirectory string) (FocusRequest, error) {
	var request FocusRequest
	err := readFocusJSON(stateDirectory, focusRequestFile, &request)
	return request, err
}

func WriteFocusAck(stateDirectory, id string) error {
	return writeFocusJSON(stateDirectory, focusAckFile, focusAck{ID: id})
}

func RequestFrontendFocus(ctx context.Context, stateDirectory string, request FocusRequest) (bool, error) {
	if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
		return false, err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return false, err
	}
	defer watcher.Close()
	if err := watcher.Add(stateDirectory); err != nil {
		return false, err
	}
	if err := WriteFocusRequest(stateDirectory, request); err != nil {
		return false, err
	}
	for {
		ack, err := readFocusAck(stateDirectory)
		if err == nil && ack.ID == request.ID {
			return true, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return false, nil
			}
			return false, ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return false, errors.New("focus acknowledgement watcher closed")
			}
			if filepath.Base(event.Name) != focusAckFile {
				continue
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return false, errors.New("focus acknowledgement watcher closed")
			}
			return false, err
		}
	}
}

func readFocusAck(stateDirectory string) (focusAck, error) {
	var ack focusAck
	err := readFocusJSON(stateDirectory, focusAckFile, &ack)
	return ack, err
}

func writeFocusJSON(stateDirectory, name string, value any) error {
	if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
		return err
	}
	contents, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(stateDirectory, name+".*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	if err := temporary.Chmod(0o600); err != nil {
		return errors.Join(err, temporary.Close(), os.Remove(temporaryName))
	}
	if _, err := temporary.Write(contents); err != nil {
		return errors.Join(err, temporary.Close(), os.Remove(temporaryName))
	}
	if err := temporary.Close(); err != nil {
		return errors.Join(err, os.Remove(temporaryName))
	}
	if err := os.Rename(temporaryName, filepath.Join(stateDirectory, name)); err != nil {
		return errors.Join(err, os.Remove(temporaryName))
	}
	return nil
}

func readFocusJSON(stateDirectory, name string, value any) error {
	contents, err := os.ReadFile(filepath.Join(stateDirectory, name))
	if err != nil {
		return err
	}
	return json.Unmarshal(contents, value)
}
