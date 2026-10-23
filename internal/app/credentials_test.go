package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialFileKeyPersistsWithPrivatePermissions(t *testing.T) {
	dataDir := t.TempDir()
	first, err := credentialFileKey(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := credentialFileKey(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || len(first) != 32 {
		t.Fatal("local credential key must persist as 32 bytes")
	}
	info, err := os.Stat(filepath.Join(dataDir, ".credential-key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("credential key permissions = %04o, want 0600", info.Mode().Perm())
	}
}
