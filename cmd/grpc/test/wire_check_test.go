package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWireProviderSets(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve runtime caller information")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	packages := []string{
		"./cmd/grpc",
		"./cmd/tasks/engagement",
		"./cmd/tasks/outbox",
	}

	for _, pkg := range packages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command("wire", "check", pkg)
			cmd.Dir = moduleRoot
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("wire check %s failed: %v\n%s", pkg, err, string(output))
			}
		})
	}
}
