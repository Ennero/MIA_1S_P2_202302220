package commands

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	stores "backend/stores"
	structures "backend/structures"
)

type MKUSR struct {
	user string
	pass string
	grp  string
}

func ParseMkusr(tokens []string) (string, error) {
	cmd := &MKUSR{}
	expectedArgs := map[string]bool{"-user": false, "-pass": false, "-grp": false}

	args := strings.Join(tokens, " ")
	re := regexp.MustCompile(`-(user|pass|grp)=("[^"]+"|[^\s]+)`)
	matches := re.FindAllStringSubmatch(args, -1)

	if len(matches) != 3 {
		return "", fmt.Errorf("número incorrecto de parámetros. Se esperan -user, -pass, -grp. Encontrados: %d", len(matches))
	}

	parsedArgs := make(map[string]bool)

	for _, match := range matches {
		key := strings.ToLower(match[1])
		value := match[2]

		if key == "user" {
			parsedArgs["-user"] = true
		}
		if key == "pass" {
			parsedArgs["-pass"] = true
		}
		if key == "grp" {
			parsedArgs["-grp"] = true
		}

		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		if len(value) > 10 {
			return "", fmt.Errorf("el valor para '-%s' ('%s') excede los 10 caracteres", key, value)
		}
		if value == "" {
			return "", fmt.Errorf("el valor para '-%s' no puede estar vacío", key)
		}

		switch key {
		case "user":
			cmd.user = value
			expectedArgs["-user"] = true
		case "pass":
			cmd.pass = value
			expectedArgs["-pass"] = true
		case "grp":
			cmd.grp = value
			expectedArgs["-grp"] = true
		default:
			return "", fmt.Errorf("parámetro desconocido detectado por regex: %s", key)
		}
	}
	for key, found := range expectedArgs {
		if !found {
			return "", fmt.Errorf("parámetro obligatorio faltante: %s", key)
		}
	}
	err := commandMkusr(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MKUSR: Usuario '%s' creado correctamente.", cmd.user), nil
}

// commandMkusr contiene la lógica principal para crear el usuario
func commandMkusr(mkusr *MKUSR) error {
	//Verificar Permisos
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando mkusr requiere inicio de sesión")
	}
	currentUser, _, partitionID := stores.Auth.GetCurrentUser()
	if currentUser != "root" {
		return fmt.Errorf("permiso denegado: solo el usuario 'root' puede ejecutar mkusr (usuario actual: %s)", currentUser)
	}

	// Obtener Partición y Superbloque
	partitionSuperblock, mountedPartition, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return fmt.Errorf("error al obtener la partición montada '%s': %w", partitionID, err)
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return errors.New("tamaño de inodo o bloque inválido en superbloque")
	}

	// Encontrar y Leer Inodo/Contenido de /users.txt
	fmt.Println("Buscando inodo para /users.txt...")
	usersInodeIndex, usersInode, errFind := structures.FindInodeByPath(partitionSuperblock, partitionPath, "/users.txt")
	if errFind != nil {
		return fmt.Errorf("error crítico: no se pudo encontrar el archivo /users.txt: %w", errFind)
	}
	if usersInode.I_type[0] != '1' {
		return errors.New("error crítico: /users.txt no es un archivo")
	}

	fmt.Println("Leyendo contenido actual de /users.txt...")
	oldContent, errRead := structures.ReadFileContent(partitionSuperblock, partitionPath, usersInode)
	if errRead != nil && oldContent == "" {
		return fmt.Errorf("error leyendo el contenido de /users.txt: %w", errRead)
	}
	if oldContent != "" && !strings.HasSuffix(oldContent, "\n") {
		oldContent += "\n"
	}

	// Parsear Contenido
	fmt.Printf("Validando usuario '%s' y grupo '%s'...\n", mkusr.user, mkusr.grp)
	lines := strings.Split(oldContent, "\n")
	highestID := int32(0)
	userExists := false
	groupExists := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		fields := strings.Split(trimmedLine, ",")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}

		if len(fields) < 3 {
			continue
		} // Formato mínimo: id,type,name

		// Rastrear ID más alto
		id64, errConv := strconv.ParseInt(fields[0], 10, 32)
		if errConv == nil {
			id := int32(id64)
			if id > highestID {
				highestID = id
			}
		}

		// Verificar si el usuario ya existe
        if len(fields) == 5 && fields[1] == "U" && strings.EqualFold(fields[3], mkusr.user) { // <-- CORRECTO: Usa índice 3 y verifica longitud 5
            userExists = true
        }

		// Verificar si el grupo existe y obtener su GID
		if fields[1] == "G" && strings.EqualFold(fields[2], mkusr.grp) {
			groupExists = true
		}
	}

	if userExists {
		return fmt.Errorf("error: el usuario '%s' ya existe", mkusr.user)
	}
	if !groupExists {
		return fmt.Errorf("error: el grupo '%s' no existe", mkusr.grp)
	}

	newUID := highestID + 1
	fmt.Printf("Nuevo UID asignado: %d. Pertenecerá al grupo '%s'.\n", newUID, mkusr.grp)

	// Preparar Nuevo Contenido
	newLine := fmt.Sprintf("%d,U,%s,%s,%s\n", newUID, mkusr.grp, mkusr.user, mkusr.pass) // Usa mkusr.grp (nombre)
	newContent := oldContent + newLine
	newSize := int32(len(newContent))
	fmt.Printf("Nuevo contenido de users.txt preparado (%d bytes).\n", newSize)

	// Liberar Bloques Antiguos de users.txt
	fmt.Println("Liberando bloques antiguos de /users.txt...")
	errFree := structures.FreeInodeBlocks(usersInode, partitionSuperblock, partitionPath)
	if errFree != nil {
		fmt.Printf("ADVERTENCIA: Error al liberar bloques antiguos de users.txt: %v. Puede haber bloques perdidos.\n", errFree)
		return fmt.Errorf("error liberando bloques antiguos: %w", errFree)
	} else {
		fmt.Println("Bloques antiguos liberados.")
	}

	// Asignar Nuevos Bloques para el nuevo contenido
	fmt.Printf("Asignando bloques para nuevo tamaño (%d bytes)...\n", newSize)
	var newAllocatedBlockIndices [15]int32
	newAllocatedBlockIndices, err = allocateDataBlocks([]byte(newContent), newSize, partitionSuperblock, partitionPath)
	if err != nil {
		return fmt.Errorf("falló la re-asignación de bloques para /users.txt: %w", err)
	}

	// Actualizar Inodo de users.txt
	fmt.Println("Actualizando inodo /users.txt...")
	usersInode.I_size = newSize
	usersInode.I_mtime = float32(time.Now().Unix())
	usersInode.I_atime = usersInode.I_mtime
	usersInode.I_block = newAllocatedBlockIndices

	usersInodeOffset := int64(partitionSuperblock.S_inode_start) + int64(usersInodeIndex)*int64(partitionSuperblock.S_inode_size)
	err = usersInode.Serialize(partitionPath, usersInodeOffset)
	if err != nil {
		return fmt.Errorf("error serializando inodo /users.txt actualizado: %w", err)
	}

	// Serializar Superbloque
	fmt.Println("Serializando SuperBlock después de MKUSR...")
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		return fmt.Errorf("error al serializar el superbloque después de mkusr: %w", err)
	}

	return nil
}
