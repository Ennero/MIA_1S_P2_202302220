package commands

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	stores "backend/stores"
	structures "backend/structures"
)

type RMUSR struct {
	user string
}

func ParseRmusr(tokens []string) (string, error) {
	cmd := &RMUSR{}

	if len(tokens) != 1 {
		return "", errors.New("formato incorrecto. Uso: rmusr -user=<nombre>")
	}

	re := regexp.MustCompile(`^-user=("[^"]+"|[^\s]+)$`)
	match := re.FindStringSubmatch(tokens[0])

	if match == nil {
		return "", fmt.Errorf("parámetro inválido o formato incorrecto: %s. Uso: rmusr -user=<nombre>", tokens[0])
	}

	value := match[1]
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		value = strings.Trim(value, "\"")
	}

	if value == "" {
		return "", errors.New("el nombre del usuario (-user) no puede estar vacío")
	}

	cmd.user = value

	err := commandRmusr(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("RMUSR: Usuario '%s' eliminado correctamente.", cmd.user), nil
}

func commandRmusr(rmusr *RMUSR) error {
	// Verificar Permisos (Root)
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando rmusr requiere inicio de sesión")
	}
	currentUser, _, partitionID := stores.Auth.GetCurrentUser()
	if currentUser != "root" {
		return fmt.Errorf("permiso denegado: solo el usuario 'root' puede ejecutar rmusr (usuario actual: %s)", currentUser)
	}

	// No permitir eliminar el usuario root
	if strings.EqualFold(rmusr.user, "root") {
		return errors.New("error: el usuario 'root' no puede ser eliminado")
	}
	// No permitir eliminar al usuario actualmente logueado
	if strings.EqualFold(rmusr.user, currentUser) && currentUser != "root" {
		return fmt.Errorf("error: no puedes eliminar al usuario '%s' mientras está logueado", currentUser)
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
	if errRead != nil {
		return fmt.Errorf("error leyendo el contenido de /users.txt: %w", errRead)
	}

	// Parsear Contenido y Validar Usuario a Eliminar
	fmt.Printf("Buscando usuario '%s' para eliminar...\n", rmusr.user)
	lines := strings.Split(oldContent, "\n")
	newLines := []string{}
	foundUser := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		fields := strings.Split(trimmedLine, ",")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}

		if len(fields) >= 4 && fields[1] == "U" && strings.EqualFold(fields[3], rmusr.user) { // <-- Cambiado fields[2] a fields[3]
			fmt.Printf("Usuario '%s' encontrado (línea: '%s'). Marcado para eliminación.\n", rmusr.user, line)
			foundUser = true
		} else {
			// Conservar la línea original
			newLines = append(newLines, line)
		}
	}
	// Verificar si se encontró el usuario
	if !foundUser {
		return fmt.Errorf("error: el usuario '%s' no fue encontrado", rmusr.user)
	}

	// Preparar Nuevo Contenido Final
	newContent := strings.Join(newLines, "\n")
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
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
	fmt.Println("Serializando SuperBlock después de RMUSR...")
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		return fmt.Errorf("error al serializar el superbloque después de rmusr: %w", err)
	}

	return nil // Éxito
}
