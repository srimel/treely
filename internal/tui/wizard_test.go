package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsGitRepo_DotGitDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = false, want true for dir with .git/", dir)
	}
}

func TestIsGitRepo_DotGitFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = false, want true for dir with .git file (linked worktree)", dir)
	}
}

func TestIsGitRepo_BareLayout(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = false, want true for bare-repo layout", dir)
	}
}

func TestIsGitRepo_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = true, want false for empty dir", dir)
	}
}

func TestIsGitRepo_PartialBareLayout(t *testing.T) {
	// HEAD only, missing objects/ and refs/ — should not be considered a repo.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = true, want false when only HEAD exists", dir)
	}
}

func TestIsGitRepo_BareLayoutObjectsIsFile(t *testing.T) {
	// objects must be a directory; a regular file at that path should disqualify.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "objects"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = true, want false when objects is a file", dir)
	}
}

func TestIsGitRepo_BareLayoutRefsIsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "refs"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isGitRepo(dir) {
		t.Errorf("isGitRepo(%q) = true, want false when refs is a file", dir)
	}
}

func TestIsGitRepo_NonExistent(t *testing.T) {
	if isGitRepo(filepath.Join(t.TempDir(), "does-not-exist")) {
		t.Error("isGitRepo on a non-existent path = true, want false")
	}
}

func TestIsTreelyCompatible_DirectRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isTreelyCompatible(dir) {
		t.Errorf("isTreelyCompatible(%q) = false, want true when dir itself is a repo", dir)
	}
}

func TestIsTreelyCompatible_BareSubdir(t *testing.T) {
	parent := t.TempDir()
	bare := filepath.Join(parent, "repo.git")
	if err := os.MkdirAll(filepath.Join(bare, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(bare, "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bare, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isTreelyCompatible(parent) {
		t.Errorf("isTreelyCompatible(%q) = false, want true when an immediate subdir is a bare repo", parent)
	}
}

func TestIsTreelyCompatible_NoRepoNearby(t *testing.T) {
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, "junk", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if isTreelyCompatible(parent) {
		t.Errorf("isTreelyCompatible(%q) = true, want false when no repo is at the dir or one level down", parent)
	}
}
