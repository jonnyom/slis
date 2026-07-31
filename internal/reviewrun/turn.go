package reviewrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type PendingTurn struct {
	RequestID         string          `json:"request_id"`
	Summary           string          `json:"summary"`
	Findings          json.RawMessage `json:"findings"`
	FindingCount      int             `json:"finding_count"`
	DeliveryCompleted bool            `json:"delivery_completed"`
}

func (s *Store) StageTurn(id string, turn PendingTurn) error {
	return s.withRunLock(id, func() error {
		if _, err := s.readRun(id); err != nil {
			return err
		}
		existing, found, err := s.readPendingTurn(id)
		if err != nil {
			return err
		}
		if found {
			if existing.RequestID == turn.RequestID {
				return nil
			}
			return fmt.Errorf("review run %q already has pending request %q", id, existing.RequestID)
		}
		return s.writePendingTurn(id, turn)
	})
}

func (s *Store) GetPendingTurn(id string) (PendingTurn, bool, error) {
	return s.readPendingTurn(id)
}

func (s *Store) readPendingTurn(id string) (PendingTurn, bool, error) {
	data, err := os.ReadFile(s.pendingTurnPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return PendingTurn{}, false, nil
	}
	if err != nil {
		return PendingTurn{}, false, err
	}
	var turn PendingTurn
	if err := json.Unmarshal(data, &turn); err != nil {
		return PendingTurn{}, false, err
	}
	return turn, true, nil
}

func (s *Store) MarkTurnDelivered(id, requestID string) error {
	return s.withRunLock(id, func() error {
		turn, found, err := s.readPendingTurn(id)
		if err != nil {
			return err
		}
		if !found {
			return os.ErrNotExist
		}
		if turn.RequestID != requestID {
			return fmt.Errorf("review run %q pending request changed from %q to %q", id, requestID, turn.RequestID)
		}
		turn.DeliveryCompleted = true
		return s.writePendingTurn(id, turn)
	})
}

func (s *Store) ClearPendingTurn(id, requestID string) error {
	return s.withRunLock(id, func() error {
		turn, found, err := s.readPendingTurn(id)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		if turn.RequestID != requestID {
			return fmt.Errorf("review run %q pending request changed from %q to %q", id, requestID, turn.RequestID)
		}
		return os.Remove(s.pendingTurnPath(id))
	})
}

func (s *Store) writePendingTurn(id string, turn PendingTurn) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(turn, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.dir, ".review-turn-*")
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
	if err := os.Rename(tempPath, s.pendingTurnPath(id)); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func (s *Store) pendingTurnPath(id string) string {
	return filepath.Join(s.dir, id+".pending-turn")
}
