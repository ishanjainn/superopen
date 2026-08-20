//go:build windows

package paths

import (
	"golang.org/x/sys/windows/registry"
)

// RemoveUserBinFromPATH strips the release-installer bin dir from the
// user-scope PATH (HKCU\Environment), matching scripts/install.ps1.
func RemoveUserBinFromPATH() []string {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()

	current, _, err := key.GetStringValue("Path")
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return nil
	}
	next, changed := stripPathList(current, pathDirsToStrip())
	if !changed {
		return nil
	}
	if err := key.SetStringValue("Path", next); err != nil {
		return nil
	}
	return []string{`HKCU\Environment\Path`}
}
