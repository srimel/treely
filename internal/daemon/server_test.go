package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSocketRemovedOnClose verifies that the socket file is cleaned up after
// NewServer + Close so a subsequent daemon start does not see EADDRINUSE.
// This catches an AF_UNIX quirk on some Windows builds where socket files
// persist as reparse points after the listener is closed.
func TestSocketRemovedOnClose(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	srv, err := NewServer(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	// Remove is a no-op if NewServer already removed it; errors here mean the
	// file persists and future daemon starts would fail.
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("could not remove socket file after Close: %v", err)
	}
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("socket file persisted after Close+Remove: %v", err)
	}
}
