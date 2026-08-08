package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func cmdIssueView(ctx context.Context, args []string) error {
	return cmdIssueViewTo(ctx, os.Stdout, args)
}

func cmdIssueViewTo(ctx context.Context, out io.Writer, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ghfa <owner/repo> issue view <num>")
	}
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue number %q", args[0])
	}
	rawURL, err := url.JoinPath(proxyBase, "gh", "repos", upstream, "issues", strconv.Itoa(number))
	if err != nil {
		return err
	}
	resp, _, status, err := do(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError(status, resp)
	}
	var envelope json.RawMessage
	if err := json.Unmarshal(resp, &envelope); err != nil {
		return fmt.Errorf("ghfa: decode issue view: %w", err)
	}
	return printJSONTo(out, envelope)
}

func cmdIssueCreate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("issue create", flag.ContinueOnError)
	title := fs.String("title", "", "issue title (required)")
	bodyFlag := fs.String("body", "", "inline markdown body")
	fileFlag := fs.String("file", "", "path to markdown file for body")
	label := fs.String("label", "", "comma-separated labels")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *title == "" {
		return fmt.Errorf("usage: ghfa <owner/repo> issue create -title <title> [-body <text> | -file <file.md>] [-label <csv>]")
	}
	if *bodyFlag != "" && *fileFlag != "" {
		return fmt.Errorf("-body and -file are mutually exclusive")
	}
	body := *bodyFlag
	if *fileFlag != "" {
		md, err := os.ReadFile(*fileFlag)
		if err != nil {
			return err
		}
		body = string(md)
	}
	var labels []string
	if *label != "" {
		for l := range strings.SplitSeq(*label, ",") {
			l = strings.TrimSpace(l)
			if l != "" {
				labels = append(labels, l)
			}
		}
	}
	rawURL, err := url.JoinPath(proxyBase, "gh", "repos", upstream, "issues")
	if err != nil {
		return err
	}
	resp, _, status, err := do(ctx, http.MethodPost, rawURL, issueRequest{
		Title:  *title,
		Body:   body,
		Labels: labels,
	})
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return statusError(status, resp)
	}
	var ref issueRef
	if err := json.Unmarshal(resp, &ref); err != nil {
		return fmt.Errorf("ghfa: decode: %w", err)
	}
	return printJSON(ref)
}

type issueRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels,omitempty"`
}

func cmdIssueEdit(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghfa <owner/repo> issue edit <num> [-title <title>] [-body <body>]")
	}
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue number %q", args[0])
	}
	fs := flag.NewFlagSet("issue edit", flag.ContinueOnError)
	title := fs.String("title", "", "new title")
	body := fs.String("body", "", "new body")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var patch issuePatch
	if *title != "" {
		patch.Title = title
	}
	if *body != "" {
		patch.Body = body
	}
	if patch.Title == nil && patch.Body == nil {
		return fmt.Errorf("at least one of -title or -body is required")
	}
	rawURL, err := url.JoinPath(proxyBase, "gh", "repos", upstream, "issues", strconv.Itoa(number))
	if err != nil {
		return err
	}
	resp, _, status, err := do(ctx, http.MethodPatch, rawURL, patch)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError(status, resp)
	}
	var ref issueRef
	if err := json.Unmarshal(resp, &ref); err != nil {
		return fmt.Errorf("ghfa: decode: %w", err)
	}
	return printJSON(ref)
}

func cmdIssueClose(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghfa <owner/repo> issue close <num> [-r completed|\"not planned\"] [-dupeof N]")
	}
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue number %q", args[0])
	}
	fs := flag.NewFlagSet("issue close", flag.ContinueOnError)
	reason := fs.String("r", "completed", "")
	dupeof := fs.Int("dupeof", 0, "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *dupeof < 0 {
		return fmt.Errorf("invalid -dupeof %d; want a positive issue number", *dupeof)
	}
	if *dupeof > 0 && *reason != "completed" {
		return fmt.Errorf("-r and -dupeof are mutually exclusive")
	}

	var patch issuePatch
	switch {
	case *dupeof > 0:
		s, r := "closed", "duplicate"
		patch = issuePatch{State: &s, StateReason: &r}
	default:
		s := "closed"
		r := strings.ReplaceAll(*reason, " ", "_")
		switch r {
		case "completed", "not_planned":
		default:
			return fmt.Errorf("invalid -r %q; want completed or \"not planned\"", *reason)
		}
		patch = issuePatch{State: &s, StateReason: &r}
	}

	rawURL, err := url.JoinPath(proxyBase, "gh", "repos", upstream, "issues", strconv.Itoa(number))
	if err != nil {
		return err
	}
	resp, _, status, err := do(ctx, http.MethodPatch, rawURL, patch)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError(status, resp)
	}
	var ref issueRef
	if err := json.Unmarshal(resp, &ref); err != nil {
		return fmt.Errorf("ghfa: decode: %w", err)
	}

	if *dupeof > 0 {
		commentURL, err := url.JoinPath(proxyBase, "gh", "repos", upstream, "issues", strconv.Itoa(number), "comments")
		if err != nil {
			return err
		}
		cresp, _, cstatus, err := do(ctx, http.MethodPost, commentURL, commentRequest{
			Body: fmt.Sprintf("Duplicate of #%d", *dupeof),
		})
		if err != nil {
			return err
		}
		if cstatus != http.StatusCreated {
			return statusError(cstatus, cresp)
		}
	}

	return printJSON(closeResult{Number: ref.Number, HTMLURL: ref.HTMLURL, State: ref.State})
}

type closeResult struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
}

func cmdIssueReopen(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghfa <owner/repo> issue reopen <num> [-c <comment>]")
	}
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue number %q", args[0])
	}
	fs := flag.NewFlagSet("issue reopen", flag.ContinueOnError)
	comment := fs.String("c", "", "reopening comment")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	s, r := "open", "reopened"
	patch := issuePatch{State: &s, StateReason: &r}
	rawURL, err := url.JoinPath(proxyBase, "gh", "repos", upstream, "issues", strconv.Itoa(number))
	if err != nil {
		return err
	}
	resp, _, status, err := do(ctx, http.MethodPatch, rawURL, patch)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError(status, resp)
	}
	var ref issueRef
	if err := json.Unmarshal(resp, &ref); err != nil {
		return fmt.Errorf("ghfa: decode: %w", err)
	}
	if *comment != "" {
		commentURL, err := url.JoinPath(proxyBase, "gh", "repos", upstream, "issues", strconv.Itoa(number), "comments")
		if err != nil {
			return err
		}
		cresp, _, cstatus, err := do(ctx, http.MethodPost, commentURL, commentRequest{Body: *comment})
		if err != nil {
			return err
		}
		if cstatus != http.StatusCreated {
			return statusError(cstatus, cresp)
		}
	}
	return printJSON(closeResult{Number: ref.Number, HTMLURL: ref.HTMLURL, State: ref.State})
}

func cmdIssueComment(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghfa <owner/repo> issue comment <num> [-body <text> | -file <file.md>]")
	}
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue number %q", args[0])
	}
	fs := flag.NewFlagSet("issue comment", flag.ContinueOnError)
	bodyFlag := fs.String("body", "", "inline markdown body")
	fileFlag := fs.String("file", "", "path to markdown file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if (*bodyFlag == "") == (*fileFlag == "") {
		return fmt.Errorf("exactly one of -body or -file is required")
	}
	body := *bodyFlag
	if *fileFlag != "" {
		md, err := os.ReadFile(*fileFlag)
		if err != nil {
			return err
		}
		body = string(md)
	}
	rawURL, err := url.JoinPath(proxyBase, "gh", "repos", upstream, "issues", strconv.Itoa(number), "comments")
	if err != nil {
		return err
	}
	resp, _, status, err := do(ctx, http.MethodPost, rawURL, commentRequest{Body: body})
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return statusError(status, resp)
	}
	var cmt comment
	if err := json.Unmarshal(resp, &cmt); err != nil {
		return fmt.Errorf("ghfa: decode comment: %w", err)
	}
	return printJSON(commentResult{Number: number})
}

type commentResult struct {
	Number int `json:"number"`
}
