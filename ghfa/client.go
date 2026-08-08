package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

var (
	proxyBase string // scheme + authority, no trailing slash
	upstream  string
)

func initClient() error {
	proxy := os.Getenv("GHFA_PROXY")
	if proxy == "" {
		return fmt.Errorf("GHFA_PROXY is required; ghfa refuses to make requests without a proxy")
	}
	proxyBase = strings.TrimRight(proxy, "/")
	return nil
}

func do(ctx context.Context, method, rawURL string, body any) ([]byte, http.Header, int, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("marshal: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, r)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "patel.codes/ghfa")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("read: %w", err)
	}
	return data, resp.Header, resp.StatusCode, nil
}

func statusError(status int, body []byte) error {
	body = body[:min(len(body), 4<<10)]
	return fmt.Errorf("ghfa: status %d: %s", status, bytes.TrimSpace(body))
}

func nextLink(header string) string {
	for part := range strings.SplitSeq(header, ",") {
		segs := strings.Split(part, ";")
		if len(segs) < 2 {
			continue
		}
		ref := strings.TrimSpace(segs[0])
		if !strings.HasPrefix(ref, "<") || !strings.HasSuffix(ref, ">") {
			continue
		}
		for _, param := range segs[1:] {
			if strings.TrimSpace(param) == `rel="next"` {
				return ref[1 : len(ref)-1]
			}
		}
	}
	return ""
}

func printJSON(v any) error {
	return printJSONTo(os.Stdout, v)
}

func printJSONTo(w io.Writer, v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(out))
	return nil
}
