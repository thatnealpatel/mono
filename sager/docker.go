package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// ensureRunning checks that the
// sage-oracle container exists
// and is running.
//
// If not, it returns an error
// describing how to start it.
func ensureRunning() error {
	cmd := exec.Command("docker", "inspect",
		"-f", "{{.State.Running}}", container)
	out, err := cmd.Output()
	if err != nil {
		if _, ok := errors.AsType[*exec.ExitError](err); ok {
			// docker inspect fails when the container
			// does not exist at all.
			return notRunningError()
		}
		return fmt.Errorf("sager: running docker inspect: %w", err)
	}
	if strings.TrimSpace(string(out)) != "true" {
		return notRunningError()
	}
	return nil
}

// notRunningError builds the user-facing error that
// explains the container is not running and how to
// start it.
func notRunningError() error {
	return fmt.Errorf(`sager: container %q is not running.
start it with:

  docker run -d --name %s \
    -v %s:%s \
    sagemath/sagemath:10.5 sleep infinity`,
		container, container, hostMount, containerMount)
}

// evalExpr runs `sage -c "<expr>"` inside the
// container and returns Sage's exit code.
func evalExpr(expr string) int {
	return passthrough("sage", "-c", expr)
}

// runScript translates the host path of a .sage file
// to its container path under the bind mount and runs
// it with `sage`. It returns the exit code.
func runScript(hostPath string) int {
	containerPath, err := translatePath(hostPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return passthrough("sage", containerPath)
}

// translatePath maps a host-side path that lives under
// hostMount to its corresponding containerMount path.
func translatePath(hostPath string) (string, error) {
	abs, err := filepath.Abs(hostPath)
	if err != nil {
		return "", fmt.Errorf("sager: resolving %q: %w", hostPath, err)
	}

	rel, err := filepath.Rel(hostMount, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf(
			"sager: %q is outside the bind mount %q; "+
				"only scripts under that directory are visible to the container",
			abs, hostMount)
	}

	// filepath.Join cleans the result; the container is
	// assumed to be a Linux path namespace, which matches
	// the host separator on the supported platform.
	return path.Join(containerMount, filepath.ToSlash(rel)), nil
}

// passthrough runs `docker exec -i <container> <args...>`
// with the caller's stdin/stdout/stderr wired straight
// through, and returns the command's exit code.
func passthrough(args ...string) int {
	full := append([]string{"exec", "-i", container}, args...)
	cmd := exec.Command("docker", full...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "sager: docker exec failed: %v\n", err)
		return 1
	}
	return 0
}
