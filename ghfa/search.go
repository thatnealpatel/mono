package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func injectQuery(r, q string) string {
	parts := []string{"repo:" + r}
	if !strings.Contains(q, "is:") {
		parts = append(parts, "is:issue")
	}
	parts = append(parts, q)
	return strings.Join(parts, " ")
}

func cmdIssueSearch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghfa <owner/repo> issue search <query>")
	}
	return searchIssues(ctx, injectQuery(upstream, strings.Join(args, " ")))
}

func cmdSearchIssues(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghfa <owner/repo> search issues <query>")
	}
	return searchIssues(ctx, strings.Join(args, " "))
}

func searchIssues(ctx context.Context, query string) error {
	params := url.Values{}
	params.Set("q", query)
	params.Set("per_page", "100")

	rawURL := apiBase + "/search/issues?" + params.Encode()
	result := &searchResult{Items: []issue{}}
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
	return printJSON(result)
}
