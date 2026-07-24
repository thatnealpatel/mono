package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"patel.codes/retrieval"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}

	var err error
	switch os.Args[1] {
	case "build":
		if os.Getenv("RETRIEVAL_HOST") != "" {
			fmt.Fprintln(os.Stderr, "goof-wiki: build is local-only")
			os.Exit(1)
		}
		err = wikiBuild()
	case "article", "links", "search":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stdout, usage)
			os.Exit(0)
		}
		query := strings.Join(os.Args[2:], " ")
		if host := os.Getenv("RETRIEVAL_HOST"); host != "" {
			err = wikiRemote(host, os.Args[1], query)
		} else {
			err = wikiLocal(os.Args[1], query)
		}
	default:
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "goof-wiki: %v\n", err)
		os.Exit(1)
	}
}

const usage = `usage: wiki <command> [args]

  build              build article index from bz2
  article <title>    article text as markdown
  links <title>      outgoing links
  search <query>     search titles
`

func wikiCacheDir() string {
	if d := os.Getenv("WIKI_CACHE_DIR"); d != "" {
		return d
	}
	base, err := os.UserCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "goof-wiki: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(base, "wiki")
}

func wikiBuild() error {
	return retrieval.BuildWiki(wikiCacheDir())
}

func wikiLocal(subcommand, query string) error {
	maxResults := 100
	if s := os.Getenv("WIKI_MAX_RESULTS"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("WIKI_MAX_RESULTS=%q: %w", s, err)
		}
		maxResults = n
	}
	w, err := retrieval.NewWiki(wikiCacheDir(), maxResults)
	if err != nil {
		return err
	}
	switch subcommand {
	case "article":
		md, err := w.Article(query)
		if err != nil {
			return err
		}
		_, err = io.WriteString(os.Stdout, md+"\n")
		return err
	case "links":
		links, err := w.Links(query)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(links)
	case "search":
		return json.NewEncoder(os.Stdout).Encode(w.Search(query))
	}
	return nil
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func wikiRemote(host, subcommand, query string) error {
	body, err := json.Marshal(struct {
		Subcommand string `json:"subcommand"`
		Query      string `json:"query"`
	}{subcommand, query})
	if err != nil {
		return err
	}
	resp, err := httpClient.Post(host+"/wikipedia", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("retrieval returned %s (body unreadable: %w)", resp.Status, err)
		}
		return fmt.Errorf("retrieval returned %s: %s", resp.Status, bytes.TrimSpace(msg))
	}
	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}
