package zmxctl

import (
	"fmt"
	"strconv"
	"strings"
)

type Session struct {
	Name     string
	PID      int
	Clients  int
	Created  int64
	StartDir string
	Command  string
	Labels   map[string]string
	Error    string
	Status   string
}

func ParseList(output string) ([]Session, error) {
	var sessions []Session
	for _, rawLine := range strings.Split(strings.TrimSpace(output), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "no sessions found in ") {
			continue
		}
		line = strings.TrimPrefix(line, "→ ")
		fields := make(map[string]string)
		for _, rawField := range strings.Split(line, "\t") {
			key, value, found := strings.Cut(strings.TrimSpace(rawField), "=")
			if !found || key == "" {
				return nil, fmt.Errorf("invalid zmx list field %q", rawField)
			}
			fields[key] = value
		}
		session := Session{
			Name:     fields["name"],
			StartDir: fields["start_dir"],
			Command:  fields["cmd"],
			Error:    fields["err"],
			Status:   fields["status"],
			Labels:   make(map[string]string),
		}
		var err error
		if session.PID, err = parseOptionalInt(fields, "pid"); err != nil {
			return nil, err
		}
		if session.Clients, err = parseOptionalInt(fields, "clients"); err != nil {
			return nil, err
		}
		if session.Created, err = parseOptionalInt64(fields, "created"); err != nil {
			return nil, err
		}
		for key, value := range fields {
			if !reservedListFields[key] {
				session.Labels[key] = value
			}
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

var reservedListFields = map[string]bool{
	"name": true, "pid": true, "clients": true, "created": true,
	"start_dir": true, "cmd": true, "err": true, "status": true,
	"ended": true, "exit_code": true,
}

func parseOptionalInt(fields map[string]string, key string) (int, error) {
	value := fields[key]
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid zmx %s %q: %w", key, value, err)
	}
	return parsed, nil
}

func parseOptionalInt64(fields map[string]string, key string) (int64, error) {
	value := fields[key]
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid zmx %s %q: %w", key, value, err)
	}
	return parsed, nil
}
