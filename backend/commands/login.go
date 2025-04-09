package commands

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	stores "backend/stores"
	structures "backend/structures"
)

type LOGIN struct {
	user string
	pass string
	id   string
}

func ParseLogin(tokens []string) (string, error) {
	cmd := &LOGIN{}
	foundParams := map[string]bool{"-user": false, "-pass": false, "-id": false} // Para rastrear requeridos

	args := strings.Join(tokens, " ")

	re := regexp.MustCompile(`-(user|pass|id)=("[^"]*"|[^\s]+)`)

	matches := re.FindAllStringSubmatch(args, -1)

	for _, match := range matches {

		keyName := strings.ToLower(match[1]) // "user", "pass", o "id"
		value := match[2]

		// Limpiar comillas del valor capturado
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		paramKey := "-" + keyName

		// Validar que el valor no esté vacío DESPUÉS de quitar comillas
		if value == "" {
			return "", fmt.Errorf("el valor para '%s' no puede estar vacío", paramKey)
		}

		switch paramKey {
		case "-user":
			if foundParams[paramKey] { // Verificar si ya se asignó
				return "", fmt.Errorf("parámetro '%s' especificado más de una vez", paramKey)
			}
			cmd.user = value
			foundParams[paramKey] = true
		case "-pass":
			if foundParams[paramKey] {
				return "", fmt.Errorf("parámetro '%s' especificado más de una vez", paramKey)
			}
			cmd.pass = value
			foundParams[paramKey] = true
		case "-id":
			if foundParams[paramKey] {
				return "", fmt.Errorf("parámetro '%s' especificado más de una vez", paramKey)
			}
			cmd.id = value
			foundParams[paramKey] = true
		default:
			// Esta parte no debería alcanzarse si la regex es correcta
			return "", fmt.Errorf("error interno del parser: clave '%s' inesperada", paramKey)
		}
	}

	missing := []string{}
	if !foundParams["-user"] {
		missing = append(missing, "-user")
	}
	if !foundParams["-pass"] {
		missing = append(missing, "-pass")
	}
	if !foundParams["-id"] {
		missing = append(missing, "-id")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("faltan parámetros requeridos: %s", strings.Join(missing, ", "))
	}

	// Llamar a la lógica principal
	err := commandLogin(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("LOGIN: Sesión iniciada para usuario '%s' en partición '%s'.", cmd.user, cmd.id), nil
}

func commandLogin(login *LOGIN) error {
	// Verificar si ya hay una sesión activa
	if stores.Auth.IsAuthenticated() {
		_, _, currentPartition := stores.Auth.GetCurrentUser()
		if currentPartition == login.id {
			return fmt.Errorf("ya hay una sesión activa en la partición '%s' para el usuario '%s'", login.id, stores.Auth.Username)
		} else {
			return fmt.Errorf("ya hay una sesión activa en otra partición ('%s'). Debes hacer 'logout' primero", currentPartition)
		}
	}

	// Obtener la partición montada y el superbloque
	partitionSuperblock, _, partitionPath, err := stores.GetMountedPartitionSuperblock(login.id)
	if err != nil {
		_, exists := stores.MountedPartitions[login.id]
		if !exists {
			return fmt.Errorf("la partición con id '%s' no está montada", login.id)
		}
		return fmt.Errorf("error al obtener la partición montada '%s': %w", login.id, err)
	}
	if partitionSuperblock.S_magic != 0xEF53 {
		return fmt.Errorf("la partición '%s' no tiene un sistema de archivos EXT2 válido (magic number incorrecto)", login.id)
	}

	// Leer /users.txt
	fmt.Println("Buscando y leyendo /users.txt...")
	_, usersInode, errFind := structures.FindInodeByPath(partitionSuperblock, partitionPath, "/users.txt")
	if errFind != nil {
		return fmt.Errorf("error crítico: no se pudo encontrar el archivo /users.txt: %w", errFind)
	}
	if usersInode.I_type[0] != '1' {
		return errors.New("error crítico: /users.txt no es un archivo")
	}

	content, errRead := structures.ReadFileContent(partitionSuperblock, partitionPath, usersInode)
	if errRead != nil {
		return fmt.Errorf("error leyendo el contenido de /users.txt: %w", errRead)
	}
	fmt.Println("Contenido leído de /users.txt.")

	// Verificar si el contenido está vacío
	lines := strings.Split(content, "\n")
	foundUser := false
	var storedPassword string

	// Buscar el usuario en las líneas
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		fields := strings.Split(trimmedLine, ",")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}

		if len(fields) == 5 && fields[1] == "U" {
			fileUsername := fields[3]
			filePassword := fields[4]

			if strings.EqualFold(fileUsername, login.user) {
				foundUser = true
				storedPassword = filePassword
				fmt.Printf("Usuario '%s' encontrado.\n", login.user)
				break
			}
		}
	}

	// Verificar si se encontró el usuario
	if !foundUser {
		return fmt.Errorf("el usuario '%s' no existe en la partición '%s'", login.user, login.id)
	}

	// Verificar la contraseña
	fmt.Println("Verificando contraseña...")
	if storedPassword != login.pass { // Comparación exacta (case-sensitive) para contraseñas
		return fmt.Errorf("contraseña incorrecta para el usuario '%s'", login.user)
	}

	// Si la validación es exitosa, establecer el estado de autenticación
	fmt.Println("Login exitoso.")
	stores.Auth.Login(login.user, login.pass, login.id)

	return nil
}
