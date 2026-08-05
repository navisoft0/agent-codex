// Package scan implements the supply-chain checks run on skill content.
// Skills execute with the agent's full permissions, so every add/update
// treats the incoming content as untrusted input: prose is checked for
// prompt-injection and secrecy patterns, scripts for download-and-execute
// payloads. Heuristics are deliberately conservative — findings are flags
// for review, not verdicts.
package scan

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Severity levels: "high" findings fail `shu scan` in CI; "warn" findings
// are informational.
const (
	High = "high"
	Warn = "warn"
)

// Finding is one flagged location inside a skill directory.
type Finding struct {
	File     string `json:"file"` // path relative to the skill directory
	Line     int    `json:"line,omitempty"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Snippet  string `json:"snippet,omitempty"`
}

type rule struct {
	name     string
	severity string
	re       *regexp.Regexp
}

// textRules apply to every text file — a skill's prose is an instruction
// stream for the agent, so injection lives here.
var textRules = []rule{
	{"prompt-injection", High, regexp.MustCompile(`(?i)\b(ignore|disregard|forget)\b.{0,20}\b(previous|prior|above|earlier|system)\b.{0,20}\b(instructions?|prompts?|rules?)`)},
	{"secrecy-instruction", High, regexp.MustCompile(`(?i)\bdo n[o']t (tell|show|reveal|mention|disclose)\b.{0,30}\b(user|human|operator)`)},
	{"exfiltration-hint", High, regexp.MustCompile(`(?i)\b(exfiltrate|send|post|upload)\b.{0,40}\b(credentials?|secrets?|tokens?|api.?keys?|\.env)\b`)},
	{"hidden-comment-instruction", Warn, regexp.MustCompile(`(?i)<!--.{0,120}\b(ignore|instruction|system prompt|jailbreak)\b`)},
}

// scriptRules additionally apply to anything executable-looking.
var scriptRules = []rule{
	{"remote-exec", High, regexp.MustCompile(`(?i)\b(curl|wget)\b[^\n|;]{0,120}\|\s*(ba|z|da)?sh\b`)},
	{"base64-exec", High, regexp.MustCompile(`(?i)base64\s+(-d|--decode)[^\n]{0,80}\|\s*(ba|z|da)?sh\b`)},
	{"eval-dynamic", Warn, regexp.MustCompile(`(?i)\beval\b\s*[("\$]`)},
	{"chmod-exec", Warn, regexp.MustCompile(`chmod\s+(\+x|0?7[0-7][0-7])`)},
}

// Dir scans one skill directory and returns findings sorted by file/line.
func Dir(dir string) ([]Finding, error) {
	var findings []Finding
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}

		if bytes.IndexByte(b, 0) >= 0 {
			sev := Warn
			if isScript(rel, b) {
				sev = High
			}
			findings = append(findings, Finding{
				File: rel, Rule: "binary-payload", Severity: sev,
				Snippet: fmt.Sprintf("binary content, %d bytes", len(b)),
			})
			return nil
		}

		rules := textRules
		if isScript(rel, b) {
			rules = append(append([]rule{}, textRules...), scriptRules...)
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, r := range rules {
				if r.re.MatchString(line) {
					findings = append(findings, Finding{
						File: rel, Line: i + 1, Rule: r.name, Severity: r.severity,
						Snippet: snippet(line),
					})
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

// HighCount returns how many findings are high severity.
func HighCount(fs []Finding) int {
	n := 0
	for _, f := range fs {
		if f.Severity == High {
			n++
		}
	}
	return n
}

func isScript(rel string, b []byte) bool {
	if strings.HasPrefix(rel, "scripts/") || strings.Contains(rel, "/scripts/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".sh", ".bash", ".zsh", ".py", ".js", ".ts", ".rb", ".ps1", ".pl":
		return true
	}
	return bytes.HasPrefix(b, []byte("#!"))
}

func snippet(line string) string {
	s := strings.TrimSpace(line)
	if len(s) > 100 {
		s = s[:100] + "…"
	}
	return s
}
