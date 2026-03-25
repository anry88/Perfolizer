package ui

import (
	"errors"
	"fmt"
)

const (
	aiSecretService       = "com.github.anry88.perfolizer.ai"
	openAIAPIKeySecretKey = "openai_api_key"
)

var (
	errSecretNotFound    = errors.New("secret not found")
	errSecretUnsupported = errors.New("secure storage unavailable")
)

type aiSecretStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	BackendName() string
	Available() bool
}

type unsupportedAISecretStore struct {
	reason string
}

func newAISecretStore() aiSecretStore {
	return newPlatformAISecretStore()
}

func (s unsupportedAISecretStore) Get(string) (string, error) {
	return "", fmt.Errorf("%w: %s", errSecretUnsupported, s.reason)
}

func (s unsupportedAISecretStore) Set(string, string) error {
	return fmt.Errorf("%w: %s", errSecretUnsupported, s.reason)
}

func (s unsupportedAISecretStore) Delete(string) error {
	return fmt.Errorf("%w: %s", errSecretUnsupported, s.reason)
}

func (s unsupportedAISecretStore) BackendName() string {
	if s.reason == "" {
		return "Unavailable"
	}
	return s.reason
}

func (s unsupportedAISecretStore) Available() bool {
	return false
}
