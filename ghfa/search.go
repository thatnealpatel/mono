package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func cmdSearchIssues(ctx context.Context, args []string) error {
	return cmdSearchIssuesTo(ctx, os.Stdout, args)
}

func cmdSearchIssuesTo(ctx context.Context, out io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghfa search issues <query>")
	}
	return searchIssues(ctx, out, strings.Join(args, " "))
}

func searchIssues(ctx context.Context, out io.Writer, query string) error {
	params := url.Values{}
	params.Set("q", query)
	params.Set("per_page", "100")

	base, err := url.JoinPath(proxyBase, "gh", "search", "issues")
	if err != nil {
		return err
	}
	rawURL := base + "?" + params.Encode()
	result := &searchResult{Items: []json.RawMessage{}}
	for first := true; rawURL != ""; first = false {
		resp, header, status, err := do(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return statusError(status, resp)
		}
		var page searchResult
		if err := json.Unmarshal(resp, &page); err != nil {
			return fmt.Errorf("ghfa: decode search: %w", err)
		}
		if first {
			result.TotalCount = page.TotalCount
			result.IncompleteResults = page.IncompleteResults
		}
		result.Items = append(result.Items, page.Items...)
		rawURL = nextLink(header.Get("Link"))
	}
	return printJSONTo(out, result)
}
