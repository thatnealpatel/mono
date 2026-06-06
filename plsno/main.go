package main

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"syscall"
	"time"
)

func main() {
	in, err := input()
	if err != nil {
		respond("deny", "plsno: "+err.Error())
		os.Exit(1)
	}
	if msg, denied := evaluate(in); denied {
		respond("deny", msg)
		os.Exit(1)
	}
	respond("approve", "")
}

// input is cosmetic wrapper to ensure
// that reads are bounded and do not
// hang when invoked incorrectly.
func input() (toolInput, error) {
	syscall.SetNonblock(int(os.Stdin.Fd()), true)
	stdin := os.NewFile(os.Stdin.Fd(), "/dev/stdin")
	if err := stdin.SetReadDeadline(time.Now().Add(5 * time.Second)); errors.Is(err, os.ErrNoDeadline) {
	} else if err != nil {
		return toolInput{}, err
	}
	var in toolInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return toolInput{}, err
	}
	return in, nil
}

type toolInput struct {
	Command string `json:"command"`
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func evaluate(in toolInput) (string, bool) {
	for _, r := range rules {
		switch r.scope {
		case cmdOnly:
			if r.pat.MatchString(in.Command) {
				return r.msg, true
			}
		case pathOnly:
			if r.pat.MatchString(in.Pattern) || r.pat.MatchString(in.Path) {
				return r.msg, true
			}
		}
	}
	return "", false
}

func respond(decision, message string) {
	json.NewEncoder(os.Stdout).Encode(struct {
		Decision string `json:"decision"`
		Message  string `json:"message,omitempty"`
	}{decision, message})
}

var rules = []rule{
	{
		regexp.MustCompile(`\b(?:u?grep|bfs|find)\b.*\.lake`),
		"Do NOT grep/find/bfs over .lake/. Use the /leandoc skill instead.",
		cmdOnly,
	},
	{
		regexp.MustCompile(`\bfind\s+(?:/(?:\s|$)|~(?:\s|$)|/home(?:\s|$))`),
		"Do NOT scan the root or home filesystem. Return to dispatcher asking for better scoping.",
		cmdOnly,
	},
	{
		regexp.MustCompile(`\bfind\s+/(?:usr|var|etc|opt|tmp|proc|sys|dev|run|boot|mnt|media|srv)\b`),
		"Do NOT scan system directories. Return to dispatcher asking for better scoping.",
		cmdOnly,
	},
	{
		regexp.MustCompile(`\bls\s+(?:-[a-zA-Z]*)?\s*/\s*$`),
		"Do NOT scan system directories. Return to dispatcher asking for better scoping.",
		cmdOnly,
	},
	{
		regexp.MustCompile(`^/(\*|$)`),
		"Do NOT glob/search from the root filesystem. Return to dispatcher asking for better scoping.",
		pathOnly,
	},
	{
		regexp.MustCompile(`^~(/|$)`),
		"Do NOT glob/search from the home directory. Return to dispatcher asking for better scoping.",
		pathOnly,
	},
	{
		regexp.MustCompile(`^/(?:usr|var|etc|opt|tmp|proc|sys|dev|run|boot|mnt|media|srv)(?:/|$)`),
		"Do NOT glob/search system directories. Return to dispatcher asking for better scoping.",
		pathOnly,
	},
	{
		regexp.MustCompile(`\.lake(?:/|$)`),
		"Do NOT glob/search .lake/. Use the /leandoc skill instead.",
		pathOnly,
	},
}

type rule struct {
	pat   *regexp.Regexp
	msg   string
	scope scope
}

type scope int

const (
	cmdOnly scope = iota
	pathOnly
)
