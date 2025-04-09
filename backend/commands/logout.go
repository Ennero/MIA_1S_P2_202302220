package commands

import (
	stores "backend/stores"
	"errors"
)

func ParseLogout(tokens []string) (string, error) {
	if len(tokens) != 0 {
		return "", errors.New("el comando logout no acepta parámetros")
	}
	// Verifica si hay una sesión activa

	if !stores.Auth.IsAuthenticated() {
		return "", errors.New("no hay ninguna sesión activa")
	}

	// Cierra la sesión
	stores.Auth.Logout()
	return "Sesión terminada", nil
}
