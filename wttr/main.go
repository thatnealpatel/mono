// Package main implements a thin
// wrapper around https://wttr.in
// to replace ANSI codes for light
// mode terminals.
//
// Many of the https://wttr.in/:help
// options are not supported here.
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
)

func main() {
	log.SetFlags(0)
	var location string
	if len(os.Args) > 1 {
		location = os.Args[1]
	}
	body, err := fetch(location)
	if err != nil {
		log.Fatalf("wttr: %v", err)
	}
	fmt.Print(string(lightMode(body)))
}

func fetch(location string) ([]byte, error) {
	url := "https://wttr.in/" + location + "?m"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "curl/8.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func lightMode(data []byte) []byte {
	return colorRe.ReplaceAllFunc(data, func(match []byte) []byte {
		sub := colorRe.FindSubmatch(match)
		n, err := strconv.Atoi(string(sub[1]))
		if err != nil {
			return match
		}
		if rep, ok := lightRemap[n]; ok {
			return []byte(fmt.Sprintf("\x1b[38;5;%dm", rep))
		}
		return match
	})
}

var (
	colorRe = regexp.MustCompile(`\x1b\[38;5;(\d+)m`)

	// wttr.in 256-color → light-mode-friendly 256-color.
	// Keys that wash out on white backgrounds get darkened
	// while preserving hue.
	lightRemap = map[int]int{
		226: 178, // bright yellow → dark gold
		220: 172, // yellow → orange-gold
		190: 142, // yellow-green → olive
		250: 243, // light gray → medium gray
		154: 70,  // lime → forest green
		118: 34,  // green → dark green
		82:  28,  // bright green → deep green
		111: 25,  // sky blue → dark blue
		240: 236, // gray → darker gray
	}
)
