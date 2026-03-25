package ui

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type linuxSecretToolStore struct{}

func newPlatformAISecretStore() aiSecretStore {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return unsupportedAISecretStore{reason: "GNOME Secret Service (secret-tool) is unavailable"}
	}
	return linuxSecretToolStore{}
}

func (linuxSecretToolStore) Get(key string) (string, error) {
	command := exec.Command("secret-tool", "lookup", "service", aiSecretService, "account", key)
	output, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			return "", errSecretNotFound
		}
		return "", fmt.Errorf("read secret-tool secret: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (linuxSecretToolStore) Set(key, value string) error {
	command := exec.Command("secret-tool", "store", "--label=Perfolizer AI Secret", "service", aiSecretService, "account", key)
	command.Stdin = strings.NewReader(value)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("store secret-tool secret: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (linuxSecretToolStore) Delete(key string) error {
	command := exec.Command("secret-tool", "clear", "service", aiSecretService, "account", key)
	output, err := command.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			if bytes.Contains(bytes.ToLower(output), []byte("not found")) {
				return errSecretNotFound
			}
			return errSecretNotFound
		}
		return fmt.Errorf("delete secret-tool secret: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (linuxSecretToolStore) BackendName() string {
	return "Secret Service"
}

func (linuxSecretToolStore) Available() bool {
	return true
}
