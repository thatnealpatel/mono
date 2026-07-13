package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type ileanRefKey struct {
	C struct {
		M string `json:"m"`
		N string `json:"n"`
	} `json:"c"`
}

func harvestIlean(data []byte) (map[string]string, error) {
	var file struct {
		Module     string                     `json:"module"`
		Decls      map[string]json.RawMessage `json:"decls"`
		References map[string]json.RawMessage `json:"references"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	names := make(map[string]string, len(file.Decls)+len(file.References))
	for name := range file.Decls {
		names[name] = file.Module
	}
	for key := range file.References {
		var ref ileanRefKey
		if err := json.Unmarshal([]byte(key), &ref); err != nil {
			return nil, fmt.Errorf("decode reference key %q: %w", key, err)
		}
		if _, ok := names[ref.C.N]; !ok {
			names[ref.C.N] = ref.C.M
		}
	}
	return names, nil
}

func walkIleanNames(root string) (map[string]string, error) {
	names := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".ilean") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fileNames, err := harvestIlean(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for name, mod := range fileNames {
			if _, ok := names[name]; !ok {
				names[name] = mod
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}
