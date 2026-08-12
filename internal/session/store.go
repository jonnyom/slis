package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/sys/unix"
)

var ErrGroupNotFound = errors.New("session group not found")

type Tab struct {
	ID              string  `json:"id"`
	Kind            TabKind `json:"kind"`
	Title           string  `json:"title"`
	CWD             string  `json:"cwd"`
	PersistenceName string  `json:"persistence_name,omitempty"`
}

type DeliveryStatus string

const (
	DeliveryPrepared DeliveryStatus = "prepared"
	DeliverySent     DeliveryStatus = "sent"
)

type Delivery struct {
	ID     string         `json:"id"`
	TabID  string         `json:"tab_id"`
	Status DeliveryStatus `json:"status"`
}

type Group struct {
	ID          string              `json:"id"`
	ActiveTabID string              `json:"active_tab_id"`
	Tabs        []Tab               `json:"tabs"`
	Deliveries  map[string]Delivery `json:"deliveries,omitempty"`
}

type Store struct {
	directory string
}

func OpenStore(stateDirectory string) *Store {
	return &Store{directory: filepath.Join(stateDirectory, "sessions")}
}

func (store *Store) Save(group Group) error {
	return store.withLock(func() error {
		groups, err := store.readAll()
		if err != nil {
			return err
		}
		groups[group.ID] = group
		return store.writeAll(groups)
	})
}

func (store *Store) Get(id string) (Group, error) {
	var group Group
	err := store.withLock(func() error {
		groups, err := store.readAll()
		if err != nil {
			return err
		}
		var found bool
		group, found = groups[id]
		if !found {
			return fmt.Errorf("%w: %s", ErrGroupNotFound, id)
		}
		return nil
	})
	return group, err
}

func (store *Store) List() ([]Group, error) {
	var result []Group
	err := store.withLock(func() error {
		groups, err := store.readAll()
		if err != nil {
			return err
		}
		result = make([]Group, 0, len(groups))
		for _, group := range groups {
			result = append(result, group)
		}
		sort.Slice(result, func(left, right int) bool {
			return result[left].ID < result[right].ID
		})
		return nil
	})
	return result, err
}

func (store *Store) Update(id string, update func(*Group) error) error {
	return store.withLock(func() error {
		groups, err := store.readAll()
		if err != nil {
			return err
		}
		group, found := groups[id]
		if !found {
			return fmt.Errorf("%w: %s", ErrGroupNotFound, id)
		}
		if err := update(&group); err != nil {
			return err
		}
		groups[id] = group
		return store.writeAll(groups)
	})
}

func (store *Store) Upsert(id string, update func(*Group) error) error {
	return store.withLock(func() error {
		groups, err := store.readAll()
		if err != nil {
			return err
		}
		group, found := groups[id]
		if !found {
			group = Group{ID: id}
		}
		if err := update(&group); err != nil {
			return err
		}
		groups[id] = group
		return store.writeAll(groups)
	})
}

func (store *Store) Delete(id string) error {
	return store.withLock(func() error {
		groups, err := store.readAll()
		if err != nil {
			return err
		}
		if _, found := groups[id]; !found {
			return fmt.Errorf("%w: %s", ErrGroupNotFound, id)
		}
		delete(groups, id)
		return store.writeAll(groups)
	})
}

func (store *Store) withLock(action func() error) error {
	if err := os.MkdirAll(store.directory, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(store.directory, "groups.lock"), os.O_CREATE|os.O_RDWR, 0o600)
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

func (store *Store) withTerminalLock(groupID, tabID string, action func() error) error {
	if err := os.MkdirAll(store.directory, 0o700); err != nil {
		return err
	}
	name := PersistenceName(groupID, tabID) + ".lock"
	lock, err := os.OpenFile(filepath.Join(store.directory, name), os.O_CREATE|os.O_RDWR, 0o600)
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

func (store *Store) readAll() (map[string]Group, error) {
	contents, err := os.ReadFile(filepath.Join(store.directory, "groups.json"))
	if os.IsNotExist(err) {
		return make(map[string]Group), nil
	}
	if err != nil {
		return nil, err
	}
	groups := make(map[string]Group)
	if err := json.Unmarshal(contents, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (store *Store) writeAll(groups map[string]Group) error {
	contents, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(store.directory, ".groups-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Rename(temporaryPath, filepath.Join(store.directory, "groups.json")); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}
