// Package app es la cáscara Wails: expone el núcleo de correo al frontend.
package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// DataDir devuelve el directorio de datos: $XDG_DATA_HOME/postacontrol o
// ~/.local/share/postacontrol.
func DataDir() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "postacontrol"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("app: no se pudo ubicar el directorio personal: %w", err)
	}
	return filepath.Join(home, ".local", "share", "postacontrol"), nil
}
