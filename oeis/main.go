// Package main implements a client for the OEIS database.
//
// When RETRIEVAL_HOST is set, commands POST to the
// supermarket retrieval endpoint. Otherwise, a local
// clone of oeisdata is queried via patel.codes/retrieval.
//
// Upstream is polled for updates at most once per day
// (local mode only).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"patel.codes/retrieval"
)

var (
	cacheDir string
	oeisDir  string
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	var err error
	switch os.Args[1] {
	case "show":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		err = run("show", os.Args[2])
	case "search":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		err = run("search", strings.Join(os.Args[2:], " "))
	case "match":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		err = run("match", strings.Join(os.Args[2:], " "))
	default:
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "goof-oeis: %v\n", err)
		os.Exit(1)
	}
}

const usage = `usage: oeis <command> [args]

  show <AXXXXXX>     sequence entry as JSON
  search <query>     search sequence names
  match <1,2,3,...>  find sequences containing terms
`

func run(subcommand, query string) error {
	if host := os.Getenv("RETRIEVAL_HOST"); host != "" {
		return oeisRemote(&http.Client{Timeout: 30 * time.Second}, host, subcommand, query)
	}
	return oeisLocal(subcommand, query)
}

var errRemoteStatus = errors.New("non-OK response from retrieval host")

func oeisRemote(client *http.Client, host, subcommand, query string) error {
	body, err := json.Marshal(struct {
		Subcommand string `json:"subcommand"`
		Query      string `json:"query"`
	}{subcommand, query})
	if err != nil {
		return err
	}
	resp, err := client.Post(host+"/oeis", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("%w: %s (body unreadable: %v)", errRemoteStatus, resp.Status, err)
		}
		return fmt.Errorf("%w: %s: %s", errRemoteStatus, resp.Status, bytes.TrimSpace(msg))
	}
	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}

func oeisLocal(subcommand, query string) error {
	cacheDir = os.Getenv("OEIS_CACHE_DIR")
	if cacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		cacheDir = filepath.Join(base, "oeis")
	}
	oeisDir = filepath.Join(cacheDir, "oeisdata.git", "seq")

	if err := ensureRepo(); err != nil {
		return err
	}

	o, err := retrieval.NewOeis(oeisDir, 50)
	if err != nil {
		return err
	}

	switch subcommand {
	case "show":
		entry, err := o.Show(query)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(entry)
	case "search":
		result := o.Search(query)
		fmt.Fprintf(os.Stderr, "%d results\n", result.Results)
		return json.NewEncoder(os.Stdout).Encode(result)
	case "match":
		matches := o.Match(query)
		fmt.Fprintf(os.Stderr, "%d results\n", len(matches))
		return json.NewEncoder(os.Stdout).Encode(matches)
	}
	return nil
}

const oeisRemoteURL = "git@github.com:oeis/oeisdata.git"

func ensureRepo() error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	dir := filepath.Join(cacheDir, "oeisdata.git")
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return ensureFresh(dir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", oeisRemoteURL, dir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cloning oeisdata: %w", err)
	}
	return nil
}

func ensureFresh(dir string) error {
	marker := filepath.Join(cacheDir, "fetched")
	if b, err := os.ReadFile(marker); err == nil {
		var ts int64
		if _, err := fmt.Sscanf(string(b), "%d", &ts); err == nil {
			if time.Since(time.Unix(ts, 0)) < 24*time.Hour {
				return nil
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	fetch := exec.CommandContext(ctx, "git", "-C", dir, "fetch")
	fetch.Stderr = os.Stderr
	if err := fetch.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "oeis: fetch failed, using cached data")
		return nil
	}
	local, err := gitRev(dir, "HEAD")
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	remote, err := gitRev(dir, "@{u}")
	if err != nil {
		return fmt.Errorf("git rev-parse @{u}: %w", err)
	}
	if local == remote {
		return nil
	}
	short := func(s string) string {
		if len(s) > 8 {
			return s[:8]
		}
		return s
	}
	fmt.Fprintf(os.Stderr, "oeis: updating %s..%s ... ", short(local), short(remote))
	pull := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only")
	pull.Stderr = os.Stderr
	if err := pull.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "update failed, using cached data")
		return nil
	}
	if err := os.WriteFile(marker, fmt.Appendf(nil, "%d\n", time.Now().Unix()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "oeis: writing marker: %v\n", err)
	}
	fmt.Fprintln(os.Stderr, "done")
	return nil
}

func gitRev(dir, ref string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", ref).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
