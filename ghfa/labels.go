package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func cmdLabelList(ctx context.Context, args []string) error {
	rawURL, err := url.JoinPath(proxyBase, "gh", "repos", upstream, "labels")
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
	return printJSON(all)
}
