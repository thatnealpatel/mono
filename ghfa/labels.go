package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func cmdLabelList(ctx context.Context, proxy, repo string, out io.Writer, args []string) error {
	rawURL, err := url.JoinPath(proxy, "gh", "repos", repo, "labels")
	if err != nil {
		return err
	}
	rawURL += "?per_page=100"
	all := []label{}
	for rawURL != "" {
		resp, header, status, err := do(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return statusError(status, resp)
		}
		var page []label
		if err := json.Unmarshal(resp, &page); err != nil {
			return fmt.Errorf("ghfa: decode labels: %w", err)
		}
		all = append(all, page...)
		rawURL = nextLink(header.Get("Link"))
	}
	return writeJSON(out, all)
}
