package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/proc"
	"github.com/jonnyom/slis/internal/zmxctl"
)

var ErrTabNotFound = errors.New("session tab not found")
var ErrTerminalMissing = errors.New("session terminal not found")
var ErrTerminalBusy = errors.New("session terminal is busy")
var ErrDeliveryIndeterminate = errors.New("session delivery result is indeterminate")

type Manager struct {
	store  *Store
	client *zmxctl.Client
}

func NewManager(store *Store, client *zmxctl.Client) *Manager {
	return &Manager{store: store, client: client}
}

func (manager *Manager) Group(groupID string) (Group, error) {
	return manager.store.Get(groupID)
}

func (manager *Manager) Ensure(ctx context.Context, groupID string, members []model.SliceMember, options LayoutOptions) (Group, error) {
	specs := PlanGroupTabs(members, options)
	desired := Group{ID: groupID, Tabs: make([]Tab, 0, len(specs))}
	for _, spec := range specs {
		desired.Tabs = append(desired.Tabs, Tab{
			ID:              spec.ID,
			Kind:            spec.Kind,
			Title:           spec.Title,
			CWD:             spec.CWD,
			PersistenceName: PersistenceName(groupID, spec.ID),
		})
	}

	previous, err := manager.store.Get(groupID)
	if err != nil && !errors.Is(err, ErrGroupNotFound) {
		return Group{}, err
	}
	group := reconcileGroup(desired, previous)

	sessions, err := manager.client.List(ctx)
	if err != nil {
		return Group{}, err
	}
	existing := make(map[string]zmxctl.Session, len(sessions))
	for _, session := range sessions {
		existing[session.Name] = session
	}
	for _, tab := range group.Tabs {
		if runtimeSession, found := existing[tab.PersistenceName]; found {
			processes, processErr := proc.SliceProcs([]int{runtimeSession.PID})
			if processErr != nil {
				return Group{}, processErr
			}
			if shouldStartLoginShell(os.Getenv("SHELL"), processes) {
				if err := manager.client.StartLoginShell(ctx, tab.PersistenceName); err != nil {
					return Group{}, err
				}
			}
			continue
		}
		if err := manager.client.Ensure(ctx, tab.PersistenceName, tab.CWD); err != nil {
			return Group{}, err
		}
	}
	if err := manager.store.Upsert(groupID, func(current *Group) error {
		group = reconcileGroup(desired, *current)
		*current = group
		return nil
	}); err != nil {
		return Group{}, err
	}
	return group, nil
}

func shouldStartLoginShell(loginShell string, processes []proc.ProcInfo) bool {
	if loginShell == "" || len(processes) != 1 {
		return false
	}
	command := strings.Fields(processes[0].Cmd)
	if len(command) == 0 {
		return false
	}
	runningShell := strings.TrimPrefix(filepath.Base(command[0]), "-")
	requestedShell := strings.TrimPrefix(filepath.Base(loginShell), "-")
	return runningShell == "bash" && requestedShell != "bash"
}

func reconcileGroup(desired, previous Group) Group {
	group := desired
	group.Deliveries = previous.Deliveries
	for _, tab := range previous.Tabs {
		if dynamicTabKind(tab.Kind) && !groupHasTab(group, tab.ID) {
			group.Tabs = append(group.Tabs, tab)
		}
	}
	if groupHasTab(group, previous.ActiveTabID) {
		group.ActiveTabID = previous.ActiveTabID
	} else if len(group.Tabs) > 0 {
		group.ActiveTabID = group.Tabs[0].ID
	}
	return group
}

func (manager *Manager) EnsureTab(ctx context.Context, groupID string, spec TabSpec) (Group, error) {
	group, err := manager.store.Get(groupID)
	if err != nil {
		return Group{}, err
	}
	if spec.CWD == "" {
		for _, existing := range group.Tabs {
			if existing.ID == group.ActiveTabID {
				spec.CWD = existing.CWD
				break
			}
		}
	}
	tab := Tab{
		ID:              spec.ID,
		Kind:            spec.Kind,
		Title:           spec.Title,
		CWD:             spec.CWD,
		PersistenceName: PersistenceName(groupID, spec.ID),
	}
	for _, existing := range group.Tabs {
		if existing.ID == spec.ID {
			tab = existing
			break
		}
	}
	sessions, err := manager.client.List(ctx)
	if err != nil {
		return Group{}, err
	}
	if !containsRuntimeSession(sessions, tab.PersistenceName) {
		if err := manager.client.Ensure(ctx, tab.PersistenceName, tab.CWD); err != nil {
			return Group{}, err
		}
	}
	err = manager.store.Update(groupID, func(current *Group) error {
		for _, existing := range current.Tabs {
			if existing.ID == tab.ID {
				group = *current
				return nil
			}
		}
		current.Tabs = append(current.Tabs, tab)
		group = *current
		return nil
	})
	return group, err
}

func (manager *Manager) Send(ctx context.Context, groupID, tabID string, input []byte) error {
	tab, err := manager.tab(groupID, tabID)
	if err != nil {
		return err
	}
	return manager.client.Send(ctx, tab.PersistenceName, input)
}

func (manager *Manager) SendOnce(ctx context.Context, groupID, tabID, deliveryID string, input []byte) error {
	if deliveryID == "" {
		return errors.New("delivery ID is required")
	}
	return manager.store.withTerminalLock(groupID, tabID, func() error {
		shouldSend := false
		if err := manager.store.Update(groupID, func(group *Group) error {
			if group.Deliveries == nil {
				group.Deliveries = make(map[string]Delivery)
			}
			delivery, found := group.Deliveries[deliveryID]
			if found && delivery.Status == DeliverySent {
				return nil
			}
			if found && delivery.Status == DeliveryPrepared {
				return fmt.Errorf("%w: %s", ErrDeliveryIndeterminate, deliveryID)
			}
			group.Deliveries[deliveryID] = Delivery{ID: deliveryID, TabID: tabID, Status: DeliveryPrepared}
			shouldSend = true
			return nil
		}); err != nil {
			return err
		}
		if !shouldSend {
			return nil
		}
		if err := manager.Send(ctx, groupID, tabID, input); err != nil {
			return err
		}
		return manager.store.Update(groupID, func(group *Group) error {
			delivery := group.Deliveries[deliveryID]
			delivery.Status = DeliverySent
			group.Deliveries[deliveryID] = delivery
			return nil
		})
	})
}

func (manager *Manager) History(ctx context.Context, groupID, tabID string, vt bool) (string, error) {
	tab, err := manager.tab(groupID, tabID)
	if err != nil {
		return "", err
	}
	return manager.client.History(ctx, tab.PersistenceName, vt)
}

func (manager *Manager) CaptureGroup(ctx context.Context, groupID string) (string, error) {
	group, err := manager.store.Get(groupID)
	if err != nil {
		return "", err
	}
	var capture strings.Builder
	for _, tab := range group.Tabs {
		history, err := manager.client.History(ctx, tab.PersistenceName, false)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&capture, "── %s ──\n", tab.Title)
		capture.WriteString(history)
		if !strings.HasSuffix(history, "\n") {
			capture.WriteByte('\n')
		}
		capture.WriteByte('\n')
	}
	return capture.String(), nil
}

func (manager *Manager) Processes(ctx context.Context, groupID, tabID string) ([]proc.ProcInfo, error) {
	_, processes, err := manager.terminalProcesses(ctx, groupID, tabID)
	return processes, err
}

func (manager *Manager) terminalProcesses(ctx context.Context, groupID, tabID string) (int, []proc.ProcInfo, error) {
	tab, err := manager.tab(groupID, tabID)
	if err != nil {
		return 0, nil, err
	}
	sessions, err := manager.client.List(ctx)
	if err != nil {
		return 0, nil, err
	}
	for _, runtimeSession := range sessions {
		if runtimeSession.Name == tab.PersistenceName {
			processes, processErr := proc.SliceProcs([]int{runtimeSession.PID})
			return runtimeSession.PID, processes, processErr
		}
	}
	return 0, nil, fmt.Errorf("%w: %s/%s", ErrTerminalMissing, groupID, tabID)
}

func (manager *Manager) Busy(ctx context.Context, groupID, tabID string) (bool, error) {
	rootPID, _, err := manager.terminalProcesses(ctx, groupID, tabID)
	if err != nil {
		return false, err
	}
	return proc.TerminalHasForegroundCommand(rootPID)
}

func (manager *Manager) StartCommand(ctx context.Context, groupID string, spec TabSpec, command string) error {
	if _, err := manager.EnsureTab(ctx, groupID, spec); err != nil {
		return err
	}
	return manager.startCommand(ctx, groupID, spec.ID, command)
}

func (manager *Manager) StartExistingCommand(ctx context.Context, groupID, tabID, command string) error {
	if _, err := manager.tab(groupID, tabID); err != nil {
		return err
	}
	return manager.startCommand(ctx, groupID, tabID, command)
}

func (manager *Manager) startCommand(ctx context.Context, groupID, tabID, command string) error {
	return manager.store.withTerminalLock(groupID, tabID, func() error {
		busy, err := manager.Busy(ctx, groupID, tabID)
		if err != nil {
			return err
		}
		if busy {
			return fmt.Errorf("%w: %s/%s", ErrTerminalBusy, groupID, tabID)
		}
		tab, err := manager.tab(groupID, tabID)
		if err != nil {
			return err
		}
		running, err := manager.client.ShellCommandRunning(tab.PersistenceName)
		if err != nil {
			return err
		}
		if running {
			return fmt.Errorf("%w: %s/%s", ErrTerminalBusy, groupID, tabID)
		}
		statePath, err := manager.client.StartShellCommand(ctx, tab.PersistenceName, command)
		if err != nil {
			return err
		}
		deadline := time.NewTimer(3 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				running, err := manager.client.ShellCommandRunning(tab.PersistenceName)
				if err != nil {
					return err
				}
				if running {
					return nil
				}
				state, readErr := os.ReadFile(statePath)
				if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
					return readErr
				}
				if strings.Contains(string(state), "done") {
					return nil
				}
			case <-deadline.C:
				return fmt.Errorf("session command was not accepted by %s/%s", groupID, tabID)
			}
		}
	})
}

func (manager *Manager) Activate(groupID, tabID string) error {
	return manager.store.Update(groupID, func(group *Group) error {
		if !groupHasTab(*group, tabID) {
			return fmt.Errorf("%w: %s/%s", ErrTabNotFound, groupID, tabID)
		}
		group.ActiveTabID = tabID
		return nil
	})
}

func (manager *Manager) AttachCommand(ctx context.Context, groupID, tabID string) (*exec.Cmd, error) {
	tab, err := manager.tab(groupID, tabID)
	if err != nil {
		return nil, err
	}
	return manager.client.AttachCommand(ctx, tab.PersistenceName), nil
}

func (manager *Manager) Kill(ctx context.Context, groupID string) error {
	group, err := manager.store.Get(groupID)
	if err != nil {
		return err
	}
	var killErrors []error
	for _, tab := range group.Tabs {
		if err := manager.client.Kill(ctx, tab.PersistenceName); err != nil {
			killErrors = append(killErrors, err)
		}
	}
	if len(killErrors) > 0 {
		return errors.Join(killErrors...)
	}
	return manager.store.Delete(groupID)
}

func (manager *Manager) KillTab(ctx context.Context, groupID, tabID string) error {
	return manager.store.withTerminalLock(groupID, tabID, func() error {
		group, err := manager.store.Get(groupID)
		if err != nil {
			return err
		}
		closedIndex := -1
		var closed Tab
		for index, tab := range group.Tabs {
			if tab.ID == tabID {
				closedIndex = index
				closed = tab
				break
			}
		}
		if closedIndex < 0 {
			return fmt.Errorf("%w: %s/%s", ErrTabNotFound, groupID, tabID)
		}
		if err := manager.client.Kill(ctx, closed.PersistenceName); err != nil {
			return err
		}
		if len(group.Tabs) == 1 {
			return manager.store.Delete(groupID)
		}
		return manager.store.Update(groupID, func(current *Group) error {
			currentIndex := -1
			for index, tab := range current.Tabs {
				if tab.ID == tabID {
					currentIndex = index
					break
				}
			}
			if currentIndex < 0 {
				return fmt.Errorf("%w: %s/%s", ErrTabNotFound, groupID, tabID)
			}
			current.Tabs = append(current.Tabs[:currentIndex], current.Tabs[currentIndex+1:]...)
			for deliveryID, delivery := range current.Deliveries {
				if delivery.TabID == tabID {
					delete(current.Deliveries, deliveryID)
				}
			}
			if current.ActiveTabID == tabID {
				nextIndex := currentIndex - 1
				if nextIndex < 0 {
					nextIndex = 0
				}
				current.ActiveTabID = current.Tabs[nextIndex].ID
			}
			return nil
		})
	})
}

func (manager *Manager) tab(groupID, tabID string) (Tab, error) {
	group, err := manager.store.Get(groupID)
	if err != nil {
		return Tab{}, err
	}
	for _, tab := range group.Tabs {
		if tab.ID == tabID {
			return tab, nil
		}
	}
	return Tab{}, fmt.Errorf("%w: %s/%s", ErrTabNotFound, groupID, tabID)
}

func groupHasTab(group Group, tabID string) bool {
	for _, tab := range group.Tabs {
		if tab.ID == tabID {
			return true
		}
	}
	return false
}

func containsRuntimeSession(sessions []zmxctl.Session, name string) bool {
	for _, session := range sessions {
		if session.Name == name {
			return true
		}
	}
	return false
}

func dynamicTabKind(kind TabKind) bool {
	return kind == TabKindAgent || kind == TabKindShell || kind == TabKindReview
}
