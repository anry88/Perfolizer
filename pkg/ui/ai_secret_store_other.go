//go:build !darwin && !linux

package ui

func newPlatformAISecretStore() aiSecretStore {
	return unsupportedAISecretStore{reason: "secure OS secret storage is not implemented on this platform yet"}
}
