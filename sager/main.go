// Package main implements a thin,
// safer wrapper around docker calls
// to a SageMath container.
//
// To start a compatible container:
//
//		docker run -d --name sage-oracle \
//		  -v /home/neal/p/proofs/Sager:/workspace \
//	   sagemath/sagemath:10.5 sleep infinity
//
// There is no proof that this program
// provides the necessary isolation
// to prevent running arbitrary commands
// in docker.
package main

import (
	"fmt"
	"os"
)

const (
	container      = "sage-oracle"
	hostMount      = "/home/neal/p/proofs/Sager"
	containerMount = "/workspace"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches on the CLI arguments and returns
// the process exit code. It mirrors Sage's exit
// code in the passthrough modes.
func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}

	if err := ensureRunning(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	switch args[0] {
	case "-c":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "sager: -c requires exactly one expression argument")
			fmt.Fprintln(os.Stderr, usage)
			return 2
		}
		return evalExpr(args[1])
	default:
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "sager: script mode takes exactly one .sage path")
			fmt.Fprintln(os.Stderr, usage)
			return 2
		}
		return runScript(args[0])
	}
}

const usage = `usage:
  sager -c "<expr>"     evaluate a Sage expression
  sager <script.sage>   run a .sage file under the bind mount`
