// Package main provides a dumb-cache
// layer around fetching RFC documents.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func main() {
	log.SetFlags(0)

	dir := os.Getenv("RFC_CACHE_DIR")
	if dir == "" {
		log.Fatal("RFC_CACHE_DIR is not set")
	}

	fs := flag.NewFlagSet("rfc", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stdout, "usage: rfc [<number>]\n\n")
		fmt.Fprintf(os.Stdout, "  (no args)     list downloaded RFCs\n")
		fmt.Fprintf(os.Stdout, "  <number>      print RFC text (downloads if needed)\n")
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}

	switch fs.NArg() {
	case 0:
		if err := rfcList(dir); err != nil {
			log.Fatal(err)
		}
	case 1:
		arg := fs.Arg(0)
		num := strings.TrimPrefix(strings.ToLower(arg), "rfc")
		if _, err := strconv.Atoi(num); err != nil {
			log.Fatalf("invalid RFC number: %s", arg)
		}
		if err := rfcShow(dir, num); err != nil {
			log.Fatal(err)
		}
	default:
		fs.Usage()
		os.Exit(1)
	}
}

func rfcList(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("no rfcs directory: %w", err)
	}
	var nums []int
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "rfc") && strings.HasSuffix(name, ".txt") {
			n, err := strconv.Atoi(name[3 : len(name)-4])
			if err == nil {
				nums = append(nums, n)
			}
		}
	}
	slices.Sort(nums)
	fmt.Printf("%d RFCs available:\n", len(nums))
	for i, n := range nums {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Printf("%d", n)
		if (i+1)%10 == 0 {
			fmt.Println()
		}
	}
	if len(nums)%10 != 0 {
		fmt.Println()
	}
	return nil
}

func rfcShow(dir, num string) error {
	if err := rfcEnsure(dir, num); err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(dir, "rfc"+num+".txt"))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(os.Stdout, f)
	return err
}

func rfcEnsure(dir, num string) error {
	path := filepath.Join(dir, "rfc"+num+".txt")
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	url := "https://www.rfc-editor.org/rfc/rfc" + num + ".txt"
	fmt.Fprintf(os.Stderr, "downloading rfc%s.txt ... ", num)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR")
		return fmt.Errorf("fetching rfc %s: %w", num, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, resp.Status)
		return fmt.Errorf("rfc %s: HTTP %s", num, resp.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, resp.Body)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
		fmt.Fprintln(os.Stderr, "ERROR")
		return fmt.Errorf("writing rfc %s: %w", num, err)
	}
	fmt.Fprintf(os.Stderr, "OK (%d bytes)\n", n)
	return nil
}
