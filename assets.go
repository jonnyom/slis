// Package slis exposes assets embedded at the module root — the agent skill and
// the agent contract doc. Only the root package can embed paths outside
// internal/ (cmd/slis is package main), so subpackages that install the skill
// import these strings from here.
package slis

import _ "embed"

// SkillMarkdown is the slis agent skill (skills/slis/SKILL.md), installed into
// each harness's skills directory by `slis init-skill`.
//
//go:embed skills/slis/SKILL.md
var SkillMarkdown string

// AgentDoc is the agent contract (docs/AGENT.md), installed alongside the skill
// as references/AGENT.md.
//
//go:embed docs/AGENT.md
var AgentDoc string
