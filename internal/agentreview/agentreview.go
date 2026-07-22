package agentreview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
)

const Schema = `{"type":"object","additionalProperties":false,"properties":{"comments":{"type":"array","items":{"type":"object","additionalProperties":false,"properties":{"repo":{"type":"string"},"file":{"type":"string"},"line":{"type":"integer","minimum":1},"end_line":{"type":"integer","minimum":0},"side":{"type":"string","enum":["new","old"]},"body":{"type":"string"}},"required":["repo","file","line","end_line","side","body"]}}},"required":["comments"]}`

type Finding struct {
	Repo    string `json:"repo"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	EndLine int    `json:"end_line,omitempty"`
	Side    string `json:"side"`
	Body    string `json:"body"`
}

type output struct {
	Comments         []Finding `json:"comments"`
	StructuredOutput *output   `json:"structured_output"`
}

type LookPath func(string) (string, error)

type Execute func(context.Context, string, []string) ([]byte, error)

func ResolveAgent(sessions config.Sessions, name string, lookPath LookPath) (config.AgentSpec, error) {
	selectedName := name
	if selectedName == "" {
		selectedName = sessions.DefaultAgent
	}
	for _, agent := range sessions.AgentList() {
		if strings.EqualFold(agent.Name, selectedName) {
			return agent, nil
		}
	}

	known := []config.AgentSpec{
		{Name: "Claude Code", Cmd: []string{"claude"}},
		{Name: "Codex", Cmd: []string{"codex"}},
		{Name: "Gemini CLI", Cmd: []string{"gemini"}},
		{Name: "Cursor Agent", Cmd: []string{"cursor-agent"}},
		{Name: "OpenCode", Cmd: []string{"opencode"}},
	}
	for _, agent := range known {
		if !strings.EqualFold(agent.Name, selectedName) && !strings.EqualFold(agent.Cmd[0], selectedName) {
			continue
		}
		path, err := lookPath(agent.Cmd[0])
		if err != nil {
			return config.AgentSpec{}, fmt.Errorf("reviewer agent %q is not available on PATH", selectedName)
		}
		agent.Cmd[0] = path
		return agent, nil
	}
	return config.AgentSpec{}, fmt.Errorf("unknown reviewer agent %q", selectedName)
}

func Run(ctx context.Context, cwd string, slice model.Slice, agent config.AgentSpec, execute Execute) ([]Finding, error) {
	temporaryDirectory, err := os.MkdirTemp("", "slis-agent-review-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporaryDirectory)

	schemaPath := filepath.Join(temporaryDirectory, "schema.json")
	resultPath := filepath.Join(temporaryDirectory, "result.json")
	if err := os.WriteFile(schemaPath, []byte(Schema), 0o600); err != nil {
		return nil, err
	}
	command, err := Command(agent, Prompt(slice), schemaPath, resultPath)
	if err != nil {
		return nil, err
	}
	stdout, err := execute(ctx, cwd, command)
	if err != nil {
		message := strings.TrimSpace(string(stdout))
		if message == "" {
			return nil, fmt.Errorf("reviewer agent %q failed: %w", agent.Name, err)
		}
		return nil, fmt.Errorf("reviewer agent %q failed: %w: %s", agent.Name, err, message)
	}
	result, readErr := os.ReadFile(resultPath)
	if readErr == nil && len(bytes.TrimSpace(result)) > 0 {
		stdout = result
	} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, readErr
	}
	findings, err := ParseOutput(stdout)
	if err != nil {
		return nil, fmt.Errorf("parse %s review: %w", agent.Name, err)
	}
	if err := ValidateFindings(slice, findings); err != nil {
		return nil, fmt.Errorf("validate %s review: %w", agent.Name, err)
	}
	return findings, nil
}

func ExecuteCommand(ctx context.Context, cwd string, command []string) ([]byte, error) {
	process := exec.CommandContext(ctx, command[0], command[1:]...)
	process.Dir = cwd
	return process.CombinedOutput()
}

func Command(agent config.AgentSpec, prompt, schemaPath, resultPath string) ([]string, error) {
	if len(agent.Cmd) == 0 {
		return nil, fmt.Errorf("reviewer agent %q has no command", agent.Name)
	}

	binary := strings.ToLower(filepath.Base(agent.Cmd[0]))
	switch binary {
	case "claude":
		return append(append([]string{}, agent.Cmd...), "-p", "--permission-mode", "plan", "--output-format", "json", "--json-schema", Schema, prompt), nil
	case "codex":
		command := []string{agent.Cmd[0], "exec"}
		command = append(command, agent.Cmd[1:]...)
		return append(command, "--sandbox", "read-only", "--skip-git-repo-check", "--ephemeral", "--output-schema", schemaPath, "--output-last-message", resultPath, prompt), nil
	case "opencode":
		command := []string{agent.Cmd[0], "run"}
		command = append(command, agent.Cmd[1:]...)
		return append(command, prompt), nil
	case "gemini", "cursor-agent":
		return append(append([]string{}, agent.Cmd...), "-p", prompt), nil
	default:
		return nil, fmt.Errorf("unsupported reviewer agent %q (%s)", agent.Name, agent.Cmd[0])
	}
}

func ParseOutput(raw []byte) ([]Finding, error) {
	for start := bytes.IndexByte(raw, '{'); start >= 0; {
		var decoded output
		if err := json.NewDecoder(bytes.NewReader(raw[start:])).Decode(&decoded); err == nil {
			if decoded.StructuredOutput != nil {
				return decoded.StructuredOutput.Comments, nil
			}
			if decoded.Comments != nil {
				return decoded.Comments, nil
			}
		}
		next := bytes.IndexByte(raw[start+1:], '{')
		if next < 0 {
			break
		}
		start += next + 1
	}
	return nil, fmt.Errorf("reviewer agent did not return the required JSON object")
}

func ValidateFindings(slice model.Slice, findings []Finding) error {
	for index, finding := range findings {
		if _, ok := slice.Members[finding.Repo]; !ok {
			return fmt.Errorf("finding %d targets unknown repo %q", index+1, finding.Repo)
		}
		cleanFile := filepath.Clean(finding.File)
		if finding.File == "" || filepath.IsAbs(finding.File) || cleanFile == ".." || strings.HasPrefix(cleanFile, ".."+string(filepath.Separator)) {
			return fmt.Errorf("finding %d has invalid file %q", index+1, finding.File)
		}
		if finding.Line < 1 {
			return fmt.Errorf("finding %d has invalid line %d", index+1, finding.Line)
		}
		if finding.EndLine > 0 && finding.EndLine < finding.Line {
			return fmt.Errorf("finding %d has end line before its start line", index+1)
		}
		if finding.Side != "new" && finding.Side != "old" {
			return fmt.Errorf("finding %d has invalid side %q", index+1, finding.Side)
		}
		if strings.TrimSpace(finding.Body) == "" {
			return fmt.Errorf("finding %d has an empty body", index+1)
		}
	}
	return nil
}

func Prompt(slice model.Slice) string {
	repos := slice.Repos()
	sort.Strings(repos)

	var builder strings.Builder
	fmt.Fprintf(&builder, "Review the complete stack diff for slis slice %q. Inspect every repository below in read-only mode. Find only concrete correctness, security, data-loss, or regression risks introduced by the stack. Do not modify files. Anchor each finding to a changed line. Return only one JSON object matching this schema: %s\n\n", slice.Name, Schema)
	for _, repo := range repos {
		member := slice.Members[repo]
		fmt.Fprintf(&builder, "- repo=%q branch=%q worktree=%q\n", repo, member.Branch, member.WorktreePath)
	}
	return builder.String()
}
