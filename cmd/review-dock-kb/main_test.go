package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gofrs/flock"
)

// These tests exercise the cron entrypoint: exit codes, locking and the
// offline commands (extract/build/verify/zero-token) that never use GitHub.

func TestCronEntrypointOfflineCommands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir) // keep ~/.claude/skills inside the temp dir
	t.Setenv("PATH", t.TempDir())
	args := func(a ...string) []string { return append([]string{"--data-dir", dir}, a...) }

	for _, c := range [][]string{
		args("repos", "add", "odoo/odoo"),
		args("extract"),
		args("build"),
		args("skill", "verify"),
		args("build"), // unchanged rebuild still succeeds
		args("zero-token"),
		args("--json", "status"),
	} {
		if code := run(c); code != 0 {
			t.Fatalf("%v exited %d", c, code)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "odoo-pr-review", "SKILL.md")); err != nil {
		t.Fatalf("skill not published to the agent directory: %v", err)
	}
	if code := run(args("rules", "approve", "R-9999")); code != 1 {
		t.Fatalf("approving a missing rule exited %d, want 1", code)
	}
	if code := run(args("nonsense")); code != 1 {
		t.Fatalf("unknown command exited %d", code)
	}
}

func TestCronEntrypointReportsLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	lock := flock.New(filepath.Join(dir, "knowledge.lock"))
	if ok, err := lock.TryLock(); !ok || err != nil {
		t.Fatal("cannot take lock", err)
	}
	defer lock.Unlock()
	for _, c := range [][]string{{"--data-dir", dir, "build"}, {"--data-dir", dir, "extract"}, {"--data-dir", dir, "run"}} {
		if code := run(c); code != 3 {
			t.Fatalf("%v with lock held exited %d, want 3", c, code)
		}
	}
}
