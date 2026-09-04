package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	environmentDomain = "sagedoc-environment-v1"
	fingerprintDomain = "sagedoc-fingerprint-v2"

	fingerprintExecutable    byte = 1
	fingerprintDistributions byte = 2
	fingerprintConda         byte = 3
	fingerprintDirectory     byte = 4
	fingerprintRegular       byte = 5
	fingerprintAbsent        byte = 6
	fingerprintPresent       byte = 7
)

func environmentFingerprint(env sageEnvironment) string {
	h := sha256.New()
	writeFramedString(h, environmentDomain)
	writeFramedString(h, env.Executable)
	writeFramedString(h, env.Prefix)
	return hex.EncodeToString(h.Sum(nil))
}

// contentFingerprint identifies every input that can affect an index: this
// executable, installed-package evidence, and the complete Sage package tree.
// The optional final argument keeps the older test helper call shape useful
// while allowing discovery's complete conda inventory to participate.
func contentFingerprint(executable, sageRoot string, distributions []distributionRecord, condaInventory ...[]condaRecord) (string, error) {
	h := sha256.New()
	writeFramedString(h, fingerprintDomain)

	writeType(h, fingerprintExecutable)
	if err := writeFramedFile(h, executable, -1); err != nil {
		return "", fmt.Errorf("fingerprint executable %q: %w", executable, err)
	}

	distributions = append([]distributionRecord(nil), distributions...)
	sort.Slice(distributions, func(i, j int) bool {
		return distributionLess(distributions[i], distributions[j])
	})
	writeType(h, fingerprintDistributions)
	writeUint64(h, uint64(len(distributions)))
	for _, distribution := range distributions {
		writeFramedString(h, distribution.Name)
		writeFramedString(h, distribution.Version)
		writeFramedString(h, distribution.Location)
		writeFramedString(h, distribution.MetadataPath)
		writeOptionalString(h, distribution.Record)
		writeOptionalString(h, distribution.DirectURL)
	}

	var conda []condaRecord
	if len(condaInventory) > 0 {
		conda = append(conda, condaInventory[0]...)
	}
	sort.Slice(conda, func(i, j int) bool { return condaRecordLess(conda[i], conda[j]) })
	writeType(h, fingerprintConda)
	writeUint64(h, uint64(len(conda)))
	for _, record := range conda {
		writeFramedString(h, record.Path)
		writeFramedString(h, record.Content)
	}

	entries, err := treeEntries(sageRoot)
	if err != nil {
		return "", fmt.Errorf("walk Sage package tree %q: %w", sageRoot, err)
	}
	for _, entry := range entries {
		path := filepath.Join(sageRoot, filepath.FromSlash(entry))
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("fingerprint Sage entry %q: %w", entry, err)
		}
		writeFramedString(h, entry)
		switch mode := info.Mode(); {
		case mode.IsDir():
			writeType(h, fingerprintDirectory)
			writeUint64(h, 0)
		case mode.IsRegular():
			writeType(h, fingerprintRegular)
			if err := writeFramedFile(h, path, info.Size()); err != nil {
				return "", fmt.Errorf("fingerprint Sage file %q: %w", entry, err)
			}
		case mode&os.ModeSymlink != 0:
			return "", fmt.Errorf("fingerprint Sage entry %q: symlinks are unsupported", entry)
		default:
			return "", fmt.Errorf("fingerprint Sage entry %q: unsupported file mode %s", entry, mode)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func treeEntries(root string) ([]string, error) {
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			if !entry.IsDir() {
				return fmt.Errorf("root is not a directory")
			}
			return nil
		}
		if entry.IsDir() && entry.Name() == "__pycache__" {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			ext := filepath.Ext(entry.Name())
			if ext == ".pyc" || ext == ".pyo" {
				source := strings.TrimSuffix(path, ext) + ".py"
				if _, err := os.Lstat(source); err == nil {
					return nil
				} else if !os.IsNotExist(err) {
					return fmt.Errorf("inspect source for bytecode %q: %w", path, err)
				}
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	return entries, nil
}

func writeFramedFile(h hash.Hash, path string, expectedSize int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	if expectedSize < 0 {
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return statErr
		}
		if !info.Mode().IsRegular() {
			_ = file.Close()
			return fmt.Errorf("unsupported file mode %s", info.Mode())
		}
		expectedSize = info.Size()
	}
	if expectedSize < 0 {
		_ = file.Close()
		return fmt.Errorf("negative file size")
	}
	writeUint64(h, uint64(expectedSize))
	copied, copyErr := io.CopyN(h, file, expectedSize)
	if copyErr == nil {
		var extra [1]byte
		if count, readErr := file.Read(extra[:]); count != 0 || (readErr != nil && readErr != io.EOF) {
			copyErr = fmt.Errorf("file changed while hashing")
		}
	}
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("read %d of %d bytes: %w", copied, expectedSize, copyErr)
	}
	return closeErr
}

func writeOptionalString(h hash.Hash, value *string) {
	if value == nil {
		writeType(h, fingerprintAbsent)
		return
	}
	writeType(h, fingerprintPresent)
	writeFramedString(h, *value)
}

func writeType(h hash.Hash, value byte) {
	_, _ = h.Write([]byte{value})
}

func writeFramedString(h hash.Hash, value string) {
	writeFramedBytes(h, []byte(value))
}

func writeFramedBytes(h hash.Hash, value []byte) {
	writeUint64(h, uint64(len(value)))
	_, _ = h.Write(value)
}

func writeUint64(h hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = h.Write(encoded[:])
}
