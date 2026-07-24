package retrieval

import (
	"compress/bzip2"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dustin/go-wikiparse"
	"patel.codes/ranking"
)

// BuildWiki produces the on-disk files that [NewWiki] requires
// from a raw dump directory.
func BuildWiki(dir string) error {
	date, err := resolveWikiDump(dir)
	if err != nil {
		return err
	}

	entries, err := buildWikiIndex(dir, date)
	if err != nil {
		return err
	}
	return buildWikiIDF(dir, date, entries)
}

func buildWikiIndex(dir, date string) ([]wikiEntry, error) {
	idxSrc := filepath.Join(dir, "enwiki-"+date+"-pages-articles-multistream-index.txt.bz2")
	f, err := os.Open(idxSrc)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ir := wikiparse.NewIndexReader(bzip2.NewReader(f))
	var entries []wikiEntry
	for {
		ie, err := ir.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading index stream: %w", err)
		}
		entries = append(entries, wikiEntry{Title: ie.ArticleName, Offset: ie.StreamOffset})
	}
	slices.SortFunc(entries, func(a, b wikiEntry) int {
		return strings.Compare(a.Title, b.Title)
	})

	idxPath := filepath.Join(dir, "enwiki-"+date+".index")
	out, err := os.Create(idxPath)
	if err != nil {
		return nil, err
	}
	if err := gob.NewEncoder(out).Encode(entries); err != nil {
		out.Close()
		return nil, err
	}
	if err := out.Close(); err != nil {
		return nil, err
	}
	return entries, nil
}

func buildWikiIDF(dir, date string, entries []wikiEntry) error {
	titles := make([]string, len(entries))
	for i, e := range entries {
		titles[i] = e.Title
	}

	idf := ranking.NewIDF(nil)
	idf.Build(titles)

	cachePath := filepath.Join(dir, "enwiki-"+date+"-idf.cache")
	f, err := os.Create(cachePath)
	if err != nil {
		return err
	}
	_, err = idf.WriteTo(f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(cachePath)
		return err
	}
	return nil
}
