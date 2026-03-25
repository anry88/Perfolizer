package ui

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type darwinKeychainStore struct{}

func newPlatformAISecretStore() aiSecretStore {
	if _, err := exec.LookPath("security"); err != nil {
		return unsupportedAISecretStore{reason: "macOS Keychain CLI is unavailable"}
	}
	return darwinKeychainStore{}
}

func (darwinKeychainStore) Get(key string) (string, error) {
	command := exec.Command("security", "find-generic-password", "-s", aiSecretService, "-a", key, "-w")
	output, err := command.Output()
	if err != nil {
		if isDarwinSecretNotFound(err) {
			return "", errSecretNotFound
		}
		return "", fmt.Errorf("read macOS Keychain secret: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (darwinKeychainStore) Set(key, value string) error {
	command := exec.Command("security", "add-generic-password", "-U", "-s", aiSecretService, "-a", key, "-w", value)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("store macOS Keychain secret: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (darwinKeychainStore) Delete(key string) error {
	command := exec.Command("security", "delete-generic-password", "-s", aiSecretService, "-a", key)
	output, err := command.CombinedOutput()
	if err != nil {
		if isDarwinSecretNotFoundOutput(output) {
			return errSecretNotFound
		}
		return fmt.Errorf("delete macOS Keychain secret: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (darwinKeychainStore) BackendName() string {
	return "macOS Keychain"
}

func (darwinKeychainStore) Available() bool {
	return true
}

func isDarwinSecretNotFound(err error) bool {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return isDarwinSecretNotFoundOutput(exitErr.Stderr)
	}
	return false
}

func isDarwinSecretNotFoundOutput(output []byte) bool {
	lower := bytes.ToLower(output)
	return bytes.Contains(lower, []byte("could not be found")) || bytes.Contains(lower, []byte("item could not be found"))
}
