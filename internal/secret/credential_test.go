package secret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRequiredPrefersSystemdCredential(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("CREDENTIALS_DIRECTORY", directory)
	t.Setenv("TEST_LEGACY_SECRET", "legado")
	if err := os.WriteFile(filepath.Join(directory, "test_secret"), []byte("segredo\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	value, err := ReadRequired("test_secret", "TEST_LEGACY_SECRET")
	if err != nil || value != "segredo" {
		t.Fatalf("credencial inesperada: valor=%q erro=%v", value, err)
	}
}

func TestReadRequiredFallsBackToEnvironment(t *testing.T) {
	t.Setenv("CREDENTIALS_DIRECTORY", "")
	t.Setenv("TEST_LEGACY_SECRET", "legado")

	value, err := ReadRequired("test_secret", "TEST_LEGACY_SECRET")
	if err != nil || value != "legado" {
		t.Fatalf("fallback inesperado: valor=%q erro=%v", value, err)
	}
}

func TestReadCredentialRejectsTraversal(t *testing.T) {
	if _, err := ReadCredential("../secret"); err == nil {
		t.Fatal("era esperado erro para travessia de diretório")
	}
}
