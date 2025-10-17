// eastercompany/dex-discord-interface/store/store.go
package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// getDexterDataPath constructs the base path for Dexter's data.
func getDexterDataPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	return filepath.Join(home, "Dexter", "discord", "server"), nil
}

// Cleanup removes the old file-based storage directory.
func Cleanup() error {
	path, err := getDexterDataPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Nothing to clean up
	}

	fmt.Println("Removing old file-based storage...")
	return os.RemoveAll(path)
}
