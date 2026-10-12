//go:build ignore

// Command scrub writes the transcript fixtures under testdata/config from
// real Claude Code sessions in ~/.claude/projects. Each fixture keeps the
// records from one uuid through another (both by prefix, empty for the
// file's start or end) and replaces prompt, answer, tool and command text
// with neutral placeholders. Structure, tags and the marker strings the
// parser keys on stay as they are.
//
// Run from the repository root: go run ./internal/transcript/testdata/scrub.go
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type fixture struct {
	name, src, from, to string
}

// sessions are relative to ~/.claude/projects.
var fixtures = []fixture{
	{"interrupt", "-Users-maxboiko-Desktop-projects-pet-herdr-go/737fef82-4e24-404c-8ce8-3418d707e2e6.jsonl", "2f4ad80e", "fa3adc4f"},
	{"skill", "-Users-maxboiko-Desktop-projects-tbtop-crm/718db0b3-e44a-4008-9dfd-a5d3efe81c60.jsonl", "6ae7714b", "8545e3c5"},
	{"commands", "-Users-maxboiko-Desktop-projects-tbtop-crm/04459cd6-7b1e-4815-8e1a-c988b1e58029.jsonl", "081a7d68", "a1fedb1c"},
	{"rewind", "-Users-maxboiko-Desktop-projects-tbtop-ecom/89976199-67fe-4eb7-800c-ae97bbb1f98b.jsonl", "068c69a9", "aff9b6ea"},
	{"parallel", "-Users-maxboiko-Desktop-projects-pet-herdr-go/1b66887f-67f6-4677-b47e-15b2873a338b.jsonl", "563cb9f4", "b8d01ffe"},
	{"notices", "-Users-maxboiko-Desktop-projects-tbtop-crm/d00a095e-7cdb-4567-80c8-81366152f67b.jsonl", "acdf7178", "f7f087a4"},
	{"queued", "-Users-maxboiko-Desktop-projects-pet-herdr-go/1b66887f-67f6-4677-b47e-15b2873a338b.jsonl", "98842e87", "dacecbb2"},
	{"error", "-private-tmp/4955a9e9-cef7-4d40-9a14-417b2cd84340.jsonl", "", ""},
	{"synthetic", "-Users-maxboiko-claude-config/98830c12-fbf8-4926-adfd-6b2b7cba52a5.jsonl", "d35b4f63", "9c31ecc7"},
	{"hooks", "-Users-maxboiko-Desktop-projects-pet-herdr-go/1b66887f-67f6-4677-b47e-15b2873a338b.jsonl", "7f4c5b59", "c048a39b"},
}

var (
	home   = os.Getenv("HOME")
	outDir = filepath.Join("internal", "transcript", "testdata", "config", "projects", "-fixture")
	// inner replaces the text of tags whose content is output or input.
	inner = regexp.MustCompile(`(?s)<(command-args|bash-input|local-command-stdout|bash-stdout|bash-stderr)>.*?</`)
)

func main() {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, f := range fixtures {
		lines, err := slice(filepath.Join(home, ".claude", "projects", f.src), f.from, f.to)
		if err != nil {
			log.Fatalf("%s: %v", f.name, err)
		}
		var out bytes.Buffer
		for _, l := range lines {
			s, err := scrubLine(l)
			if err != nil {
				log.Fatalf("%s: %v", f.name, err)
			}
			if s != nil {
				out.Write(s)
				out.WriteByte('\n')
			}
		}
		path := filepath.Join(outDir, f.name+".jsonl")
		if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %d records\n", path, len(lines))
	}
}

// slice returns the lines from the record whose uuid starts with from
// through the one starting with to. An empty bound is the file's start or end.
func slice(path, from, to string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out [][]byte
	on := from == ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		var r struct {
			UUID string `json:"uuid"`
		}
		_ = json.Unmarshal(line, &r)
		if !on && from != "" && strings.HasPrefix(r.UUID, from) {
			on = true
		}
		if on {
			out = append(out, line)
		}
		if on && to != "" && r.UUID != "" && strings.HasPrefix(r.UUID, to) {
			return out, nil
		}
	}
	return out, sc.Err()
}

// scrubLine returns the scrubbed record, or nil for a record that has no
// place in a fixture.
func scrubLine(line []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(line, &m); err != nil {
		return nil, err
	}
	typ, _ := m["type"].(string)
	if _, ok := m["uuid"]; !ok && typ != "ai-title" {
		return nil, nil
	}
	keep := map[string]any{"type": typ, "uuid": m["uuid"], "parentUuid": m["parentUuid"],
		"timestamp": m["timestamp"], "subtype": m["subtype"], "isMeta": m["isMeta"],
		"promptSource": m["promptSource"], "isApiErrorMessage": m["isApiErrorMessage"], "error": m["error"],
		"durationMs": m["durationMs"], "version": m["version"], "cwd": "/work/app", "gitBranch": "main"}
	if _, ok := m["gitBranch"]; !ok {
		delete(keep, "gitBranch")
	}
	if _, ok := m["cwd"]; !ok {
		delete(keep, "cwd")
	}
	switch typ {
	case "ai-title":
		keep["aiTitle"] = "Fixture session"
	case "user":
		msg, _ := m["message"].(map[string]any)
		keep["message"] = map[string]any{"role": "user", "content": scrubUser(msg["content"])}
		keep["isCompactSummary"] = m["isCompactSummary"]
	case "assistant":
		msg, _ := m["message"].(map[string]any)
		keep["message"] = map[string]any{"id": msg["id"], "model": msg["model"], "stop_reason": msg["stop_reason"],
			"content": scrubAssistant(msg["content"], msg["model"] == "<synthetic>")}
	case "system":
		if c, ok := m["content"].(string); ok {
			keep["content"] = scrubSystem(m["subtype"], c)
		}
	case "attachment":
		a, _ := m["attachment"].(map[string]any)
		att := map[string]any{"type": a["type"], "commandMode": a["commandMode"]}
		if p, ok := a["prompt"].(string); ok {
			att["prompt"] = scrubUserText(p)
		}
		keep["attachment"] = att
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(keep); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func scrubSystem(subtype any, c string) string {
	switch subtype {
	case "away_summary":
		return "※ recap: recap text (disable recaps in /config)"
	case "local_command":
		return "output"
	}
	return c
}

// scrubUser scrubs a user record's content, a string or blocks.
func scrubUser(content any) any {
	switch c := content.(type) {
	case string:
		return scrubUserText(c)
	case []any:
		out := make([]any, 0, len(c))
		for _, b := range c {
			blk, _ := b.(map[string]any)
			switch blk["type"] {
			case "text":
				out = append(out, map[string]any{"type": "text", "text": scrubUserText(str(blk["text"]))})
			case "tool_result":
				out = append(out, map[string]any{"type": "tool_result", "tool_use_id": blk["tool_use_id"],
					"is_error": blk["is_error"], "content": scrubResult(blk["content"])})
			case "image":
				out = append(out, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": ""}})
			}
		}
		return out
	}
	return ""
}

// scrubResult keeps the agent's own denial text and the rest become a
// placeholder.
func scrubResult(c any) any {
	s := ""
	switch v := c.(type) {
	case string:
		s = v
	case []any:
		for _, b := range v {
			if blk, ok := b.(map[string]any); ok && blk["type"] == "text" {
				s = str(blk["text"])
				break
			}
		}
	}
	if strings.HasPrefix(s, "The user doesn't want to proceed") {
		return s
	}
	return "result text"
}

// scrubAssistant keeps the thinking block empty and the tool calls' names,
// and gives inputs and answers placeholders. Synthetic messages are Claude's
// own fixed texts and stay.
func scrubAssistant(content any, synthetic bool) any {
	blks, _ := content.([]any)
	out := make([]any, 0, len(blks))
	for _, b := range blks {
		blk, _ := b.(map[string]any)
		switch blk["type"] {
		case "text":
			t := "answer text"
			if synthetic {
				t = str(blk["text"])
			}
			out = append(out, map[string]any{"type": "text", "text": t})
		case "thinking":
			out = append(out, map[string]any{"type": "thinking", "thinking": ""})
		case "tool_use":
			in, _ := blk["input"].(map[string]any)
			for k := range in {
				in[k] = "x"
			}
			out = append(out, map[string]any{"type": "tool_use", "id": blk["id"], "name": blk["name"], "input": in})
		}
	}
	return out
}

// scrubUserText replaces a user text with a placeholder, keeping the tags
// and marker strings the parser reads.
func scrubUserText(t string) string {
	switch {
	case strings.Contains(t, "<task-notification>"):
		return "<task-notification>\n<task-id>" + tagOf(t, "task-id") + "</task-id>\n<status>completed</status>\n" +
			"<summary>Agent \"task\" finished</summary>\n<result>result text</result>\n</task-notification>"
	case strings.HasPrefix(t, "<command-message>") || strings.HasPrefix(t, "<command-name>"):
		return inner.ReplaceAllStringFunc(t, func(m string) string {
			tag := regexp.MustCompile(`<([a-z-]+)>`).FindStringSubmatch(m)[1]
			return "<" + tag + ">" + placeholder(tag) + "</"
		})
	case strings.HasPrefix(t, "<bash-input>"), strings.HasPrefix(t, "<local-command-stdout>"), strings.HasPrefix(t, "<bash-stdout>"):
		return inner.ReplaceAllStringFunc(t, func(m string) string {
			tag := regexp.MustCompile(`<([a-z-]+)>`).FindStringSubmatch(m)[1]
			return "<" + tag + ">" + placeholder(tag) + "</"
		})
	case strings.HasPrefix(t, "[Request interrupted"), strings.HasPrefix(t, "<local-command-caveat>"):
		return t
	case strings.HasPrefix(t, "Base directory for this skill"):
		return "Base directory for this skill: /work/skill\n\n# Skill\n\nskill body text"
	}
	return "prompt text"
}

// placeholder is the neutral text that replaces a tag's content.
func placeholder(tag string) string {
	switch tag {
	case "command-args":
		return "args"
	case "bash-input":
		return "echo ok"
	case "bash-stderr":
		return ""
	}
	return "output"
}

func tagOf(t, name string) string {
	m := regexp.MustCompile(`<` + name + `>([^<]*)</` + name + `>`).FindStringSubmatch(t)
	if m == nil {
		return ""
	}
	return m[1]
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
