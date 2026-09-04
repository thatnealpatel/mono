package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"patel.codes/indexing"
)

const maxFingerprintBuildAttempts = 3

var (
	errFingerprintChanged = errors.New("Sage installation changed during index construction")
	errDisposableCache    = errors.New("cached index is structurally invalid")
)

type indexOptions struct {
	ConfiguredPython string
	CacheRoot        string
	Executable       string
	Diagnostics      io.Writer
}

func defaultCacheRoot() (string, error) {
	if configured := os.Getenv("SAGEDOC_CACHE_DIR"); configured != "" {
		return configured, nil
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "sagedoc"), nil
}

// openIndex discovers and fingerprints the Sage environment on every call. A
// cold or damaged cache is handled under the lock for the environment selected
// by that observation. Discovery is repeated after taking the lock and before
// publication; any retargeting restarts selection with a bounded retry count.
func openIndex(ctx context.Context, options indexOptions) (*indexing.Index, error) {
	var lastChange error
	for attempt := 1; attempt <= maxFingerprintBuildAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		env, fingerprint, environmentDir, databasePath, err := discoverSelection(ctx, options)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		index, exists, openErr := openExistingIndexContext(ctx, databasePath)
		if openErr == nil && exists {
			if err := ctx.Err(); err != nil {
				_ = index.Close()
				return nil, err
			}
			return index, nil
		}
		if openErr != nil && (!exists || !isDisposableCacheError(openErr)) {
			return nil, fmt.Errorf("open cached index %q: %w", databasePath, openErr)
		}
		// Disposable cache damage is removed only after lock
		// acquisition and revalidation. Permission and unrelated
		// I/O errors never reach here.

		lockPath := filepath.Join(environmentDir, "build.lock")
		lock, err := acquireEnvironmentLock(ctx, lockPath)
		if err != nil {
			return nil, fmt.Errorf("acquire environment build lock: %w", err)
		}

		selectedPath, selectErr := selectOrBuildIndex(ctx, options, env, fingerprint, environmentDir, attempt)
		releaseErr := releaseEnvironmentLock(lock)
		if selectErr != nil || releaseErr != nil {
			combined := errors.Join(selectErr, releaseErr)
			if errors.Is(selectErr, errFingerprintChanged) && releaseErr == nil {
				lastChange = combined
				continue
			}
			return nil, combined
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		index, exists, err = openExistingIndexContext(ctx, selectedPath)
		if err != nil {
			return nil, fmt.Errorf("open published index %q: %w", selectedPath, err)
		}
		if !exists {
			return nil, fmt.Errorf("open published index %q: %w", selectedPath, os.ErrNotExist)
		}
		return index, nil
	}
	return nil, fmt.Errorf("%w after %d attempts: %v", errFingerprintChanged, maxFingerprintBuildAttempts, lastChange)
}

func discoverSelection(ctx context.Context, options indexOptions) (sageEnvironment, string, string, string, error) {
	env, err := discover(ctx, options.ConfiguredPython)
	if err != nil {
		return sageEnvironment{}, "", "", "", err
	}
	fingerprint, err := contentFingerprint(options.Executable, env.SageRoot, env.Distributions, env.CondaRecords)
	if err != nil {
		return sageEnvironment{}, "", "", "", err
	}
	if err := ctx.Err(); err != nil {
		return sageEnvironment{}, "", "", "", err
	}
	environmentDir := filepath.Join(options.CacheRoot, environmentFingerprint(env))
	if err := os.MkdirAll(environmentDir, 0o700); err != nil {
		return sageEnvironment{}, "", "", "", fmt.Errorf("create environment cache %q: %w", environmentDir, err)
	}
	return env, fingerprint, environmentDir, filepath.Join(environmentDir, fingerprint+".db"), nil
}

// selectOrBuildIndex runs with environmentDir's lock held. One attempt starts
// with wholly fresh discovery rather than reusing the pre-lock observation.
func selectOrBuildIndex(ctx context.Context, options indexOptions, preliminary sageEnvironment, preliminaryFingerprint, environmentDir string, attempt int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	locked, err := discover(ctx, options.ConfiguredPython)
	if err != nil {
		return "", err
	}
	lockedFingerprint, err := contentFingerprint(options.Executable, locked.SageRoot, locked.Distributions, locked.CondaRecords)
	if err != nil {
		return "", changedInstallationError(attempt, "fingerprint after acquiring lock", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	lockedEnvironmentDir := filepath.Join(options.CacheRoot, environmentFingerprint(locked))
	path := filepath.Join(lockedEnvironmentDir, lockedFingerprint+".db")
	if !sameSageEnvironment(preliminary, locked) || preliminaryFingerprint != lockedFingerprint || lockedEnvironmentDir != environmentDir {
		return "", changedInstallationError(attempt, "selection changed while waiting for lock", nil)
	}

	index, exists, openErr := openExistingIndexContext(ctx, path)
	if openErr == nil && exists {
		closeErr := index.Close()
		if closeErr != nil {
			return "", fmt.Errorf("close validated index %q: %w", path, closeErr)
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return path, nil
	}
	if openErr != nil && (!exists || !isDisposableCacheError(openErr)) {
		return "", fmt.Errorf("open cached index %q: %w", path, openErr)
	}
	if exists {
		if err := os.Remove(path); err != nil {
			return "", errors.Join(
				fmt.Errorf("open cached index %q: %w", path, openErr),
				fmt.Errorf("remove invalid cached index %q: %w", path, err),
			)
		}
	}

	verify := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		after, err := discover(ctx, options.ConfiguredPython)
		if err != nil {
			return changedInstallationError(attempt, "rediscover before publication", err)
		}
		afterFingerprint, err := contentFingerprint(options.Executable, after.SageRoot, after.Distributions, after.CondaRecords)
		if err != nil {
			return changedInstallationError(attempt, "fingerprint before publication", err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		afterEnvironmentDir := filepath.Join(options.CacheRoot, environmentFingerprint(after))
		afterPath := filepath.Join(afterEnvironmentDir, afterFingerprint+".db")
		if !sameSageEnvironment(locked, after) || afterPath != path {
			return changedInstallationError(attempt, "selection changed during extraction", nil)
		}
		return nil
	}
	if err := buildIndex(ctx, options.ConfiguredPython, path, options.Diagnostics, verify); err != nil {
		return "", err
	}
	return path, nil
}

func changedInstallationError(attempt int, operation string, cause error) error {
	detail := fmt.Errorf("%w (attempt %d of %d): %s", errFingerprintChanged, attempt, maxFingerprintBuildAttempts, operation)
	if cause != nil {
		return errors.Join(detail, cause)
	}
	return detail
}

func openExistingIndex(path string) (*indexing.Index, bool, error) {
	return openExistingIndexContext(context.Background(), path)
}

func openExistingIndexContext(ctx context.Context, path string) (*indexing.Index, bool, error) {
	exists, err := existingIndex(path)
	if err != nil || !exists {
		return nil, exists, err
	}
	if err := validateCacheDatabase(ctx, path); err != nil {
		if isSQLiteStructuralError(err, true) {
			err = fmt.Errorf("%w: %v", errDisposableCache, err)
		}
		return nil, true, err
	}
	index, err := indexing.Open(path, SageTokenizer{})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		if isSQLiteStructuralError(err, false) || isSchemaVersionError(err) {
			err = fmt.Errorf("%w: %v", errDisposableCache, err)
		}
		return nil, true, err
	}
	if err := validateRequiredIndexStructure(index); err != nil {
		closeErr := index.Close()
		if isSQLiteStructuralError(err, true) {
			err = fmt.Errorf("%w: %v", errDisposableCache, err)
		}
		return nil, true, errors.Join(err, closeErr)
	}
	return index, true, nil
}

var requiredCacheSchemaObjects = map[string]string{
	"decls":       "table",
	"names":       "table",
	"fts":         "table",
	"fts_data":    "table",
	"fts_idx":     "table",
	"fts_content": "table",
	"fts_docsize": "table",
	"fts_config":  "table",
	"decls_name":  "index",
	"decls_final": "index",
	"names_final": "index",
}

// validateCacheDatabase inspects the database independently of normal query
// paths. integrity_check visits every reachable page, including pages that a
// no-hit Lookup or Search would never read, while sqlite_schema validation
// catches valid SQLite databases from which a required table or index was
// dropped.
func validateCacheDatabase(ctx context.Context, path string) (resultErr error) {
	dsn, err := sqliteFileDSN(path, url.Values{
		"immutable": {"1"},
		"mode":      {"ro"},
	})
	if err != nil {
		return fmt.Errorf("construct SQLite URI for %q: %w", path, err)
	}
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	defer func() {
		resultErr = errors.Join(resultErr, database.Close())
	}()

	rows, err := database.QueryContext(ctx, "pragma integrity_check")
	if err != nil {
		return fmt.Errorf("check SQLite integrity: %w", err)
	}
	integrityOK := false
	for rows.Next() {
		var report string
		if err := rows.Scan(&report); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read SQLite integrity report: %w", err)
		}
		if report != "ok" {
			_ = rows.Close()
			return fmt.Errorf("%w: SQLite integrity check: %s", errDisposableCache, report)
		}
		integrityOK = true
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("read SQLite integrity report: %w", err)
	}
	if !integrityOK {
		return fmt.Errorf("%w: SQLite integrity check returned no result", errDisposableCache)
	}

	rows, err = database.QueryContext(ctx, `select type, name from sqlite_schema
		where type in ('table', 'index')`)
	if err != nil {
		return fmt.Errorf("inspect SQLite schema: %w", err)
	}
	found := make(map[string]string, len(requiredCacheSchemaObjects))
	for rows.Next() {
		var objectType, name string
		if err := rows.Scan(&objectType, &name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read SQLite schema: %w", err)
		}
		if _, required := requiredCacheSchemaObjects[name]; required {
			found[name] = objectType
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("read SQLite schema: %w", err)
	}
	for name, wantType := range requiredCacheSchemaObjects {
		if gotType := found[name]; gotType != wantType {
			if gotType == "" {
				gotType = "missing"
			}
			return fmt.Errorf("%w: required SQLite schema object %q is %s, want %s", errDisposableCache, name, gotType, wantType)
		}
	}
	return nil
}

// sqliteFileDSN constructs a file URI rather than concatenating a path
// into one. In particular, ?, #, %, and & in cache paths remain path
// bytes instead of becoming URI query or fragment syntax.
func sqliteFileDSN(path string, parameters url.Values) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	uri.RawQuery = parameters.Encode()
	return uri.String(), nil
}

// These public operations also validate the columns and FTS
// behavior used by sagedoc, supplementing the whole-file and
// explicit object checks above.
func validateRequiredIndexStructure(index *indexing.Index) error {
	const probe = "sagedoccachestructureprobeuniqueterm"
	if _, err := index.Lookup(probe); err != nil {
		return fmt.Errorf("validate lookup structures: %w", err)
	}
	if _, err := index.Search(probe); err != nil {
		return fmt.Errorf("validate search structures: %w", err)
	}
	return nil
}

type sqliteErrorCoder interface {
	Code() int
}

func isSQLiteStructuralError(err error, includeGenericSchemaError bool) bool {
	var coded sqliteErrorCoder
	if !errors.As(err, &coded) {
		return false
	}
	switch coded.Code() & 0xff {
	case 11, 26: // SQLITE_CORRUPT, SQLITE_NOTADB
		return true
	case 1: // SQLITE_ERROR: missing table/column during our fixed validation.
		return includeGenericSchemaError
	default:
		return false
	}
}

func isSchemaVersionError(err error) bool {
	message := err.Error()
	return strings.Contains(message, ": schema version ") && strings.HasSuffix(message, fmt.Sprintf(", want %d", indexing.SchemaVersion))
}

func isDisposableCacheError(err error) bool {
	return errors.Is(err, errDisposableCache)
}

func existingIndex(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("inspect index %q: %w", path, err)
}
