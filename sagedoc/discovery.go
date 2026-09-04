package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed discover.py
var discoveryScript []byte

type distributionRecord struct {
	Name         string  `json:"name"`
	Version      string  `json:"version"`
	Location     string  `json:"location"`
	MetadataPath string  `json:"metadata_path"`
	Record       *string `json:"record"`
	DirectURL    *string `json:"direct_url"`
}

type condaRecord struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type sageEnvironment struct {
	Executable    string               `json:"executable"`
	Prefix        string               `json:"prefix"`
	SageRoot      string               `json:"sage_root"`
	Distributions []distributionRecord `json:"distributions"`
	CondaRecords  []condaRecord        `json:"conda_records"`
}

func discover(ctx context.Context, configuredPython string) (sageEnvironment, error) {
	if configuredPython == "" {
		return sageEnvironment{}, errors.New("SAGEDOC_PYTHON is required")
	}

	cmd := exec.CommandContext(ctx, configuredPython, "-I", "-")
	cmd.Stdin = bytes.NewReader(discoveryScript)
	var stdout bytes.Buffer
	stderr := &diagnosticCapture{limit: diagnosticLimit}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return sageEnvironment{}, commandError(configuredPython, "run Sage discovery", err, stderr.String())
	}

	var env sageEnvironment
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&env); err != nil {
		return sageEnvironment{}, commandError(configuredPython, "decode Sage discovery output", err, stderr.String())
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("more than one JSON value")
		}
		return sageEnvironment{}, commandError(configuredPython, "decode Sage discovery output", err, stderr.String())
	}

	for label, path := range map[string]string{
		"interpreter": env.Executable,
		"prefix":      env.Prefix,
		"Sage root":   env.SageRoot,
	} {
		if path == "" || !filepath.IsAbs(path) {
			return sageEnvironment{}, commandError(configuredPython, "validate Sage discovery output", fmt.Errorf("%s path %q is not absolute", label, path), stderr.String())
		}
	}
	info, err := os.Stat(env.SageRoot)
	if err != nil {
		return sageEnvironment{}, commandError(configuredPython, "validate Sage package root", err, stderr.String())
	}
	if !info.IsDir() {
		return sageEnvironment{}, commandError(configuredPython, "validate Sage package root", fmt.Errorf("%q is not a directory", env.SageRoot), stderr.String())
	}
	for _, distribution := range env.Distributions {
		if distribution.Location == "" || !filepath.IsAbs(distribution.Location) {
			return sageEnvironment{}, commandError(configuredPython, "validate distribution inventory", fmt.Errorf("distribution %q has non-absolute location %q", distribution.Name, distribution.Location), stderr.String())
		}
		if distribution.MetadataPath != "" && !filepath.IsAbs(distribution.MetadataPath) {
			return sageEnvironment{}, commandError(configuredPython, "validate distribution inventory", fmt.Errorf("distribution %q has non-absolute metadata path %q", distribution.Name, distribution.MetadataPath), stderr.String())
		}
	}
	for _, record := range env.CondaRecords {
		if record.Path == "" || filepath.IsAbs(record.Path) || filepath.Clean(record.Path) != record.Path || filepath.Base(record.Path) != record.Path {
			return sageEnvironment{}, commandError(configuredPython, "validate conda inventory", fmt.Errorf("conda record path %q is not a canonical relative filename", record.Path), stderr.String())
		}
	}
	sort.Slice(env.Distributions, func(i, j int) bool {
		return distributionLess(env.Distributions[i], env.Distributions[j])
	})
	sort.Slice(env.CondaRecords, func(i, j int) bool {
		return condaRecordLess(env.CondaRecords[i], env.CondaRecords[j])
	})
	return env, nil
}

func distributionLess(left, right distributionRecord) bool {
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	if left.Version != right.Version {
		return left.Version < right.Version
	}
	if left.Location != right.Location {
		return left.Location < right.Location
	}
	if left.MetadataPath != right.MetadataPath {
		return left.MetadataPath < right.MetadataPath
	}
	if comparison := compareOptionalString(left.Record, right.Record); comparison != 0 {
		return comparison < 0
	}
	return compareOptionalString(left.DirectURL, right.DirectURL) < 0
}

func condaRecordLess(left, right condaRecord) bool {
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	return left.Content < right.Content
}

func compareOptionalString(left, right *string) int {
	if left == nil {
		if right == nil {
			return 0
		}
		return -1
	}
	if right == nil {
		return 1
	}
	return strings.Compare(*left, *right)
}

func sameSageEnvironment(left, right sageEnvironment) bool {
	if left.Executable != right.Executable || left.Prefix != right.Prefix || left.SageRoot != right.SageRoot ||
		len(left.Distributions) != len(right.Distributions) || len(left.CondaRecords) != len(right.CondaRecords) {
		return false
	}
	for i := range left.Distributions {
		leftRecord, rightRecord := left.Distributions[i], right.Distributions[i]
		if distributionLess(leftRecord, rightRecord) || distributionLess(rightRecord, leftRecord) {
			return false
		}
	}
	for i := range left.CondaRecords {
		if left.CondaRecords[i] != right.CondaRecords[i] {
			return false
		}
	}
	return true
}

func commandError(python, operation string, err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("%s with configured Python %q: %w", operation, python, err)
	}
	return fmt.Errorf("%s with configured Python %q: %w (stderr: %s)", operation, python, err, stderr)
}
