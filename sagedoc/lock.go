package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const lockRetryInterval = 20 * time.Millisecond

func acquireEnvironmentLock(ctx context.Context, path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = file.Close()
			return nil, fmt.Errorf("flock %q: %w", path, err)
		}
		timer := time.NewTimer(lockRetryInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func releaseEnvironmentLock(file *os.File) error {
	if file == nil {
		return nil
	}
	var unlockErr error
	for {
		unlockErr = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		if !errors.Is(unlockErr, unix.EINTR) {
			break
		}
	}
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
}
