// Colocar en commands/utils.go o structures/users.go o similar
// O mantenerla privada dentro de commands/ si solo se usa ahí

package commands // O el paquete donde esté

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
	"strings"

	structures "backend/structures"
)

// UserInfo contiene UID y GID de un usuario
type UserInfo struct {
	UID int32
	GID int32
}

// Busca el UID y GID de un usuario en /users.txt
func getUserInfo(username string, sb *structures.SuperBlock, diskPath string) (int32, int32, error) {
	fmt.Printf("Buscando UID y GID para usuario '%s'...\n", username)
	if username == "" {
		return -1, -1, errors.New("getUserInfo: nombre de usuario vacío")
	}
	if sb == nil {
		return -1, -1, errors.New("getUserInfo: Superbloque nil")
	}
	if sb.S_inode_size <= 0 {
		return -1, -1, errors.New("getUserInfo: Tamaño de inodo inválido")
	}

	// Leer Inodo 1 (users.txt)
	usersInode := &structures.Inode{}
	usersInodeOffset := int64(sb.S_inode_start + 1*sb.S_inode_size)
	if err := usersInode.Deserialize(diskPath, usersInodeOffset); err != nil {
		return -1, -1, fmt.Errorf("getUserInfo: error crítico leyendo inodo 1: %w", err)
	}
	if usersInode.I_type[0] != '1' {
		return -1, -1, errors.New("getUserInfo: inodo 1 no es archivo")
	}

	// Leer contenido
	content, errRead := structures.ReadFileContent(sb, diskPath, usersInode)
	if errRead != nil {
		return -1, -1, fmt.Errorf("getUserInfo: error leyendo /users.txt: %w", errRead)
	}

	// Escanear para encontrar UID, GID y Nombre de Grupo del usuario
	scanner := bufio.NewScanner(strings.NewReader(content))
	var foundUID int32 = -1
	var userGroupName string = ""
	var foundGID int32 = -1
	userLineFound := false

	fmt.Println("  Escaneando users.txt...")
	lineNumber := 0
	// Primera pasada: encontrar línea del usuario y su grupo
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 4 {
			continue
		}

		userType := strings.TrimSpace(fields[1])
		lineUsername := strings.TrimSpace(fields[3])

		if userType == "U" && strings.EqualFold(lineUsername, username) {
			lineUIDStr := strings.TrimSpace(fields[0])
			uid64, errConv := strconv.ParseInt(lineUIDStr, 10, 32)
			if errConv != nil {
				return -1, -1, fmt.Errorf("getUserInfo: UID inválido ('%s') para '%s'", lineUIDStr, username)
			}
			foundUID = int32(uid64)
			if foundUID <= 0 {
				return -1, -1, fmt.Errorf("getUserInfo: UID inválido (%d) para '%s'", foundUID, username)
			}
			userGroupName = strings.TrimSpace(fields[2]) // Guardar nombre del grupo
			userLineFound = true
			fmt.Printf("  Línea de usuario encontrada: UID=%d, Grupo='%s'\n", foundUID, userGroupName)
			break // Encontramos al usuario, salimos de la primera pasada
		}
	}
	if err := scanner.Err(); err != nil {
		return -1, -1, fmt.Errorf("getUserInfo: error escaneando /users.txt (pasada 1): %w", err)
	}
	if !userLineFound {
		return -1, -1, fmt.Errorf("usuario '%s' no encontrado en /users.txt", username)
	}
	if userGroupName == "" {
		return -1, -1, fmt.Errorf("grupo no encontrado para usuario '%s'", username)
	} // Validación extra

	// Segunda pasada para encontrar el GID del grupo
	fmt.Printf("  Buscando GID para grupo '%s'...\n", userGroupName)
	scanner = bufio.NewScanner(strings.NewReader(content)) // Reiniciar scanner
	lineNumber = 0
	groupLineFound := false
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			continue
		} // Línea de grupo necesita GID, TIPO, NOMBRE

		lineType := strings.TrimSpace(fields[1])
		lineGroupName := strings.TrimSpace(fields[2])

		if lineType == "G" && strings.EqualFold(lineGroupName, userGroupName) {
			lineGIDStr := strings.TrimSpace(fields[0])
			gid64, errConv := strconv.ParseInt(lineGIDStr, 10, 32)
			if errConv != nil {
				return -1, -1, fmt.Errorf("getUserInfo: GID inválido ('%s') para grupo '%s'", lineGIDStr, userGroupName)
			}
			foundGID = int32(gid64)
			if foundGID <= 0 {
				return -1, -1, fmt.Errorf("getUserInfo: GID inválido (%d) para grupo '%s'", foundGID, userGroupName)
			}
			groupLineFound = true
			fmt.Printf("  Línea de grupo encontrada: GID=%d\n", foundGID)
			break // Encontramos el grupo
		}
	}
	if err := scanner.Err(); err != nil {
		return -1, -1, fmt.Errorf("getUserInfo: error escaneando /users.txt (pasada 2): %w", err)
	}
	if !groupLineFound {
		return -1, -1, fmt.Errorf("grupo '%s' (del usuario '%s') no encontrado en /users.txt", userGroupName, username)
	}

	fmt.Printf("  Información encontrada para '%s': UID=%d, GID=%d\n", username, foundUID, foundGID)
	return foundUID, foundGID, nil
}
