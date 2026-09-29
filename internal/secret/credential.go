package secret

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
)

const maximumCredentialSize = 64 * 1024

var credentialNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ReadCredential lê uma credencial entregue pelo systemd em
// CREDENTIALS_DIRECTORY. O nome nunca é interpretado como um caminho.
func ReadCredential(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !credentialNamePattern.MatchString(name) {
		return "", fmt.Errorf(i18n.Choose("nome de credencial inválido: %q", "invalid credential name: %q"), name)
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
		return "", fmt.Errorf(i18n.Choose("a credencial %s não é um arquivo regular", "credential %s is not a regular file"), name)
	}
	if info.Size() > maximumCredentialSize {
		return "", fmt.Errorf(i18n.Choose("a credencial %s excede o limite de %d bytes", "credential %s exceeds the %d-byte limit"), name, maximumCredentialSize)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// Quebras de linha nunca fazem parte de um token ou senha; removê-las nas
	// duas pontas evita cabeçalhos inválidos quando o arquivo vem com sobras.
	value := strings.Trim(string(content), "\r\n")
	if value == "" {
		return "", fmt.Errorf(i18n.Choose("a credencial %s está vazia", "credential %s is empty"), name)
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
			i18n.Choose("credencial %s não foi fornecida pelo systemd e %s não foi configurada", "credential %s was not provided by systemd and %s is not set"),
			credentialName,
			legacyEnvironmentName,
		)
	}
	return value, nil
}
