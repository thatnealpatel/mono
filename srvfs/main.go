// Command srvfs serves a directory over HTTP on 0.0.0.0:11111.
// It is intended for personal use and adds no authentication or file filtering.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: srvfs <dir>")
		os.Exit(1)
	}

	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		log.Fatalf("%s is not a directory", root)
	}

	const addr = "0.0.0.0:11111"
	log.Printf("serving %s at http://%s", root, addr)
	log.Fatal(http.ListenAndServe(addr, http.FileServer(http.Dir(root))))
}
