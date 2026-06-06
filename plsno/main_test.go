package main

import "testing"

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name string
		in   toolInput
		deny bool
	}{
		{"bash grep lake", toolInput{Command: "grep -r foo .lake/packages"}, true},
		{"bash find lake", toolInput{Command: "find .lake -name '*.lean'"}, true},
		{"bash ugrep lake", toolInput{Command: "ugrep pattern .lake/build"}, true},
		{"bash find root", toolInput{Command: "find / -name 'mmgroup'"}, true},
		{"bash find home", toolInput{Command: "find ~ -type f"}, true},
		{"bash find /home", toolInput{Command: "find /home -name '*.py'"}, true},
		{"bash find /usr", toolInput{Command: "find /usr/lib -name '*.so'"}, true},
		{"bash find /var", toolInput{Command: "find /var/log -name '*.log'"}, true},
		{"bash find /tmp", toolInput{Command: "find /tmp -name 'foo'"}, true},
		{"bash find /proc", toolInput{Command: "find /proc/1/status"}, true},
		{"bash ls root", toolInput{Command: "ls /"}, true},
		{"bash ls -la root", toolInput{Command: "ls -la /"}, true},

		{"bash find cwd", toolInput{Command: "find . -name '*.go'"}, false},
		{"bash find project", toolInput{Command: "find /home/neal/code/mono -name '*.py'"}, false},
		{"bash grep src", toolInput{Command: "grep -r pattern src/"}, false},
		{"bash ls project", toolInput{Command: "ls -la /home/neal/code/mono"}, false},
		{"bash ls bare", toolInput{Command: "ls"}, false},
		{"bash go build", toolInput{Command: "go build ./..."}, false},
		{"bash find relative", toolInput{Command: "find ./mmgroup -name '*.c'"}, false},
		{"bash find abs project", toolInput{Command: "find /home/neal/code/mono/mmgroup -type f"}, false},

		{"glob root star", toolInput{Pattern: "/**/*.go"}, true},
		{"glob root bare", toolInput{Pattern: "/"}, true},
		{"glob home tilde", toolInput{Pattern: "~"}, true},
		{"glob home subdir", toolInput{Pattern: "~/code/**"}, true},
		{"glob /usr", toolInput{Pattern: "/usr/lib/**/*.so"}, true},
		{"glob /var", toolInput{Pattern: "/var/log/**"}, true},
		{"glob /tmp", toolInput{Pattern: "/tmp/**"}, true},
		{"glob /proc", toolInput{Pattern: "/proc/**"}, true},
		{"glob /etc", toolInput{Pattern: "/etc/**"}, true},
		{"glob lake nested", toolInput{Pattern: ".lake/packages/**/*.lean"}, true},
		{"glob lake deep", toolInput{Pattern: "src/.lake/build/**"}, true},

		{"glob relative star", toolInput{Pattern: "**/*.go"}, false},
		{"glob dot relative", toolInput{Pattern: "./**/*.go"}, false},
		{"glob src", toolInput{Pattern: "src/**/*.py"}, false},
		{"glob abs project", toolInput{Pattern: "/home/neal/code/mono/**/*.go"}, false},
		{"glob abs mmgroup", toolInput{Pattern: "/home/neal/code/mono/mmgroup/**/*.py"}, false},

		{"grep path root", toolInput{Pattern: "sym", Path: "/"}, true},
		{"grep path /usr", toolInput{Pattern: "sym", Path: "/usr/lib"}, true},
		{"grep path /var", toolInput{Pattern: "sym", Path: "/var"}, true},
		{"grep path /tmp", toolInput{Pattern: "sym", Path: "/tmp"}, true},
		{"grep path tilde", toolInput{Pattern: "sym", Path: "~"}, true},
		{"grep path ~/code", toolInput{Pattern: "sym", Path: "~/code"}, true},

		{"grep path src", toolInput{Pattern: "sym", Path: "src/"}, false},
		{"grep path dot", toolInput{Pattern: "sym", Path: "."}, false},
		{"grep path project", toolInput{Pattern: "sym", Path: "/home/neal/code/mono"}, false},
		{"grep path cgt", toolInput{Pattern: "sym", Path: "/home/neal/code/mono/cgt"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, denied := evaluate(tt.in)
			if denied != tt.deny {
				t.Errorf("evaluate(%+v) = (%q, %v), want deny=%v", tt.in, msg, denied, tt.deny)
			}
		})
	}
}
