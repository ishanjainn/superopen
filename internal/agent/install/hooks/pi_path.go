package hooks

import (
	"os"
	"path/filepath"
)

// PiManagedExtensionMarker is the marker on the Pi extension Superopen already
// installs. The Pi-family installers compare against it so they do not treat
// a Pi file as one of their own.
const PiManagedExtensionMarker = "so-managed-pi-extension:v1"

// PiExtensionPath is where the existing Pi install writes its extension.
func PiExtensionPath(level Level) (string, error) {
	switch level {
	case "", LevelUser:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".pi", "agent", "extensions", "superopen", "index.ts"), nil
	case LevelProject:
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, ".pi", "extensions", "superopen", "index.ts"), nil
	default:
		return "", os.ErrInvalid
	}
}
