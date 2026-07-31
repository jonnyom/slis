package reviewrun

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type Status string

const (
	StatusQueued   Status = "queued"
	StatusRunning  Status = "running"
	StatusClean    Status = "clean"
	StatusFindings Status = "findings"
	StatusFailed   Status = "failed"
)

type Role string

const (
	RoleUser     Role = "user"
	RoleReviewer Role = "reviewer"
	RoleSystem   Role = "system"
)

type Run struct {
	ID           string    `json:"id"`
	Slice        string    `json:"slice"`
	Agent        string    `json:"agent"`
	Window       string    `json:"window"`
	Status       Status    `json:"status"`
	FindingCount int       `json:"finding_count"`
	Error        string    `json:"error,omitempty"`
	LastRequest  string    `json:"last_request,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Message struct {
	ID        string    `json:"id"`
	Role      Role      `json:"role"`
	RequestID string    `json:"request_id,omitempty"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Detail struct {
	Run
	Messages []Message `json:"messages"`
}

type Store struct {
	dir string
}

func Open(stateDir string) *Store {
	return &Store{dir: filepath.Join(stateDir, "review-runs")}
}

func (s *Store) Create(slice, agent, window string) (Run, error) {
	now := time.Now().UTC()
	id, err := newID(now)
	if err != nil {
		return Run{}, err
	}
	run := Run{
		ID: id, Slice: slice, Agent: agent, Window: window,
		Status: StatusQueued, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.writeRun(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *Store) Get(id string) (Detail, error) {
	run, err := s.readRun(id)
	if err != nil {
		return Detail{}, err
	}
	messages, err := s.readMessages(id)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Run: run, Messages: messages}, nil
}

func (s *Store) List(slice string) ([]Run, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Run{}, nil
	}
	if err != nil {
		return nil, err
	}
	runs := make([]Run, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		run, readErr := s.readRun(id)
		if readErr != nil {
			return nil, readErr
		}
		if slice == "" || run.Slice == slice {
			runs = append(runs, run)
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	return runs, nil
}

func (s *Store) ListDetails(slice string) ([]Detail, error) {
	runs, err := s.List(slice)
	if err != nil {
		return nil, err
	}
	details := make([]Detail, 0, len(runs))
	for _, run := range runs {
		messages, err := s.readMessages(run.ID)
		if err != nil {
			return nil, err
		}
		details = append(details, Detail{Run: run, Messages: messages})
	}
	return details, nil
}

func (s *Store) AppendMessage(id string, role Role, body string) (Message, error) {
	if _, err := s.readRun(id); err != nil {
		return Message{}, err
	}
	return s.appendMessage(id, Message{Role: role, Body: body})
}

func (s *Store) AppendReviewerMessage(id, requestID, body string) (Message, error) {
	if _, err := s.readRun(id); err != nil {
		return Message{}, err
	}
	messages, err := s.readMessages(id)
	if err != nil {
		return Message{}, err
	}
	for _, message := range messages {
		if message.Role == RoleReviewer && message.RequestID == requestID {
			return message, nil
		}
	}
	return s.appendMessage(id, Message{Role: RoleReviewer, RequestID: requestID, Body: body})
}

func (s *Store) appendMessage(id string, message Message) (Message, error) {
	now := time.Now().UTC()
	messageID, err := newID(now)
	if err != nil {
		return Message{}, err
	}
	message.ID = messageID
	message.CreatedAt = now
	data, err := json.Marshal(message)
	if err != nil {
		return Message{}, err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return Message{}, err
	}
	file, err := os.OpenFile(s.messagePath(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return Message{}, err
	}
	data = append(data, '\n')
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return Message{}, err
	}
	if err := file.Close(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Store) SetRunning(id string) error {
	return s.update(id, func(run *Run) {
		run.Status = StatusRunning
		run.Error = ""
	})
}

func (s *Store) SetQueued(id string) error {
	return s.update(id, func(run *Run) {
		run.Status = StatusQueued
		run.Error = ""
	})
}

func (s *Store) SetCompleted(id string, findingCount int) error {
	return s.RecordTurn(id, "", findingCount)
}

func (s *Store) SetTurnCompleted(id, lastRequest string, findingCount int) error {
	return s.RecordTurn(id, lastRequest, findingCount)
}

func (s *Store) RecordTurn(id, lastRequest string, findingCount int) error {
	return s.update(id, func(run *Run) {
		run.FindingCount += findingCount
		if lastRequest != "" {
			run.LastRequest = lastRequest
		}
		run.Error = ""
		if run.FindingCount == 0 {
			run.Status = StatusClean
		} else {
			run.Status = StatusFindings
		}
	})
}

func (s *Store) SetWindow(id, window string) error {
	return s.update(id, func(run *Run) {
		run.Window = window
	})
}

func (s *Store) SetFailed(id, message string) error {
	return s.update(id, func(run *Run) {
		run.Status = StatusFailed
		run.Error = message
	})
}

func (s *Store) update(id string, change func(*Run)) error {
	return s.withRunLock(id, func() error {
		run, err := s.readRun(id)
		if err != nil {
			return err
		}
		change(&run)
		run.UpdatedAt = time.Now().UTC()
		return s.writeRun(run)
	})
}

func (s *Store) withRunLock(id string, action func() error) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(s.runPath(id)+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return errors.Join(err, lock.Close())
	}
	actionErr := action()
	unlockErr := unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	return errors.Join(actionErr, unlockErr, lock.Close())
}

func (s *Store) readRun(id string) (Run, error) {
	data, err := os.ReadFile(s.runPath(id))
	if err != nil {
		return Run{}, err
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *Store) readMessages(id string) ([]Message, error) {
	file, err := os.Open(s.messagePath(id))
	if errors.Is(err, os.ErrNotExist) {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	messages := []Message{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var message Message
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})
	return messages, nil
}

func (s *Store) writeRun(run Run) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.dir, ".review-run-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, s.runPath(run.ID)); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func (s *Store) runPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *Store) messagePath(id string) string {
	return filepath.Join(s.dir, id+".messages.jsonl")
}

func newID(now time.Time) (string, error) {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate review id: %w", err)
	}
	return now.Format("20060102T150405.000000000") + "-" + hex.EncodeToString(random), nil
}
