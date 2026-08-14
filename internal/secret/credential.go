package secret

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maximumCredentialSize = 64 * 1024

var credentialNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ReadCredential lê uma credencial entregue pelo systemd em
// CREDENTIALS_DIRECTORY. O nome nunca é interpretado como um caminho.
func ReadCredential(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !credentialNamePattern.MatchString(name) {
		return "", fmt.Errorf("nome de credencial inválido: %q", name)
	}

	directory := strings.TrimSpace(os.Getenv("CREDENTIALS_DIRECTORY"))
	if directory == "" {
		return "", os.ErrNotExist
	}

	path := filepath.Join(directory, name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("a credencial %s não é um arquivo regular", name)
	}
	if info.Size() > maximumCredentialSize {
		return "", fmt.Errorf("a credencial %s excede o limite de %d bytes", name, maximumCredentialSize)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimRight(string(content), "\r\n")
	if value == "" {
		return "", fmt.Errorf("a credencial %s está vazia", name)
	}
	return value, nil
}

// ReadRequired usa systemd-creds como fonte principal e mantém a variável de
// ambiente apenas como compatibilidade de migração.
func ReadRequired(credentialName string, legacyEnvironmentName string) (string, error) {
	value, err := ReadCredential(credentialName)
	if err == nil {
		return value, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}

	value = os.Getenv(legacyEnvironmentName)
	if value == "" {
		return "", fmt.Errorf(
			"credencial %s não foi fornecida pelo systemd e %s não foi configurada",
			credentialName,
			legacyEnvironmentName,
		)
	}
	return value, nil
}
