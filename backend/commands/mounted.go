package commands

import (
	stores "backend/stores"
	"errors"
	"fmt"
	"strings"
)

func ParseMounted(tokens []string) (string, error) {

	if len(tokens) != 0 {
		return "", fmt.Errorf("comando mounted no recibe argumentos")
	}
	return commandMounted()
}

func commandMounted() (string, error){
	if len(stores.ListMounted) == 0 {
		return "", errors.New("no hay particiones montadas")
	}

	var sb strings.Builder
	sb.WriteString("Particiones montadas:\n")
	for _, path := range stores.ListMounted {
		sb.WriteString(path)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}