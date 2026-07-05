package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// prRef is a minimal pull request reference.
type prRef struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
}

// cmdPRCreate creates a cross-repo pull request.
// Usage: ghfa <owner/repo> pr create -title <t> -head <owner:branch> -base <branch> [-body|-file]
func cmdPRCreate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pr create", flag.ContinueOnError)
	title := fs.String("title", "", "PR title (required)")
	bodyFlag := fs.String("body", "", "inline PR body")
	fileFlag := fs.String("file", "", "path to markdown file for body")
	head := fs.String("head", "", "head branch (owner:branch for cross-repo)")
	base := fs.String("base", "", "base branch (default: main)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *title == "" || *head == "" {
		return fmt.Errorf("usage: ghfa <owner/repo> pr create -title <t> -head <owner:branch> -base <branch> [-body|-file]")
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
	baseBranch := *base
	if baseBranch == "" {
		baseBranch = "main"
	}
	if !strings.Contains(*head, ":") {
		return fmt.Errorf("-head must be in owner:branch format for cross-repo PRs")
	}

	type prRequest struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
	}
	req := prRequest{
		Title: *title,
		Body:  body,
		Head:  *head,
		Base:  baseBranch,
	}

	rawURL, err := url.JoinPath(proxyBase, "gh", "repos", upstream, "pulls")
	if err != nil {
		return err
	}
	resp, _, status, err := do(ctx, http.MethodPost, rawURL, req)
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return statusError(status, resp)
	}
	var pr prRef
	if err := json.Unmarshal(resp, &pr); err != nil {
		return fmt.Errorf("ghfa: decode PR: %w", err)
	}
	return printJSON(pr)
}
