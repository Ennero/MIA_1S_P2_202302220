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
	// Verificar Permisos 
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando rmusr requiere inicio de sesión")
	}
	currentUser, _, partitionID := stores.Auth.GetCurrentUser()
	if currentUser != "root" {
		return fmt.Errorf("permiso denegado: solo el usuario 'root' puede ejecutar rmusr (usuario actual: %s)", currentUser)
	}

	// No permitir modificar el usuario root
	if strings.EqualFold(rmusr.user, "root") {
		return errors.New("error: el usuario 'root' no puede ser modificado por rmusr")
	}

	// No permitir que root se modifique a sí mismo
	if strings.EqualFold(rmusr.user, currentUser) && currentUser == "root" {
		return errors.New("error: el usuario 'root' no puede modificarse a sí mismo con rmusr")
	}

	// Obtener Partición y Superbloque
	partitionSuperblock, mountedPartition, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return fmt.Errorf("error al obtener la partición montada '%s': %w", partitionID, err)
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return errors.New("tamaño de inodo o bloque inválido en superbloque")
	}
	if partitionSuperblock.S_magic != 0xEF53 {
		return fmt.Errorf("magia del superbloque inválida en partición '%s'", partitionID)
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

	// Parsear Contenido y Modificar Usuario
	fmt.Printf("Buscando usuario '%s' para modificar UID a 0...\n", rmusr.user)
	lines := strings.Split(oldContent, "\n")
	newLines := []string{}
	foundUser := false
	userExistsWithUIDZero := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue // Ignorar líneas vacías
		}

		fields := strings.Split(trimmedLine, ",")
		// Validar formato básico de la línea
		if len(fields) < 4 {
			fmt.Printf("Advertencia: Línea con formato incorrecto en users.txt: '%s'. Se conservará.\n", line)
			newLines = append(newLines, line)
			continue
		}

		// Limpiar espacios de cada campo
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}

		isUserLine := fields[1] == "U"                           // Verificar tipo 'U'
		isTargetUser := strings.EqualFold(fields[3], rmusr.user)
		currentUID := fields[0]

		// Verificar si el usuario ya tiene UID 0
		if isUserLine && isTargetUser && currentUID == "0" {
			fmt.Printf("Información: El usuario '%s' ya tiene UID 0.\n", rmusr.user)
			userExistsWithUIDZero = true // Marcar que ya existe con UID 0
			newLines = append(newLines, line)
			foundUser = true // Marcar como encontrado también
			continue         // Pasar a la siguiente línea
		}

		if isUserLine && isTargetUser {
			fmt.Printf("Usuario '%s' encontrado (UID: %s, Línea: '%s'). Modificando UID a 0.\n", rmusr.user, currentUID, line)
			fields[0] = "0"                           // Cambiar UID a "0"
			modifiedLine := strings.Join(fields, ",") // Reconstruir la línea
			newLines = append(newLines, modifiedLine) // Añadir la LÍNEA MODIFICADA
			foundUser = true
		} else {
			newLines = append(newLines, line)
		}
	} 

	if !foundUser {
		return fmt.Errorf("error: el usuario '%s' no fue encontrado en /users.txt", rmusr.user)
	}
	if userExistsWithUIDZero {
		fmt.Println("No se realizaron cambios en el archivo ya que el usuario ya tenía UID 0.")
		return fmt.Errorf("error: el usuario '%s' ya tiene UID 0", rmusr.user)
	}

	newContent := strings.Join(newLines, "\n")
	// Asegurar que termine con newline si no está vacío
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newSize := int32(len(newContent))
	fmt.Printf("Nuevo contenido de users.txt preparado (%d bytes).\n", newSize)

	fmt.Println("Liberando bloques antiguos de /users.txt...")
	errFree := structures.FreeInodeBlocks(usersInode, partitionSuperblock, partitionPath)
	if errFree != nil {
		fmt.Printf("ADVERTENCIA: Error al liberar bloques antiguos de users.txt: %v. Puede haber bloques perdidos.\n", errFree)
		return fmt.Errorf("error liberando bloques antiguos: %w", errFree)
	} else {
		fmt.Println("Bloques antiguos liberados.")
	}

	fmt.Printf("Asignando bloques para nuevo tamaño (%d bytes)...\n", newSize)
	var newAllocatedBlockIndices [15]int32
	newAllocatedBlockIndices, err = allocateDataBlocks([]byte(newContent), newSize, partitionSuperblock, partitionPath)
	if err != nil {
		// Falló la reasignación
		return fmt.Errorf("falló la re-asignación de bloques para /users.txt: %w", err)
	}

	// Actualizar Inodo de users.txt
	fmt.Println("Actualizando inodo /users.txt...")
	usersInode.I_size = newSize
	usersInode.I_mtime = float32(time.Now().Unix())
	usersInode.I_atime = usersInode.I_mtime // Actualizar también atime
	// Resetear los punteros antiguos y asignar los nuevos
	for k := range usersInode.I_block { usersInode.I_block[k] = -1 }
	usersInode.I_block = newAllocatedBlockIndices

	usersInodeOffset := int64(partitionSuperblock.S_inode_start) + int64(usersInodeIndex)*int64(partitionSuperblock.S_inode_size)
	err = usersInode.Serialize(partitionPath, usersInodeOffset)
	if err != nil {
		// Fallo crítico, el inodo no se actualizó
		return fmt.Errorf("error serializando inodo /users.txt actualizado: %w", err)
	}

	// Serializar Superbloque 
	fmt.Println("Serializando SuperBlock después de RMUSR...")
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		return fmt.Errorf("ADVERTENCIA: error al serializar el superbloque después de rmusr, los contadores podrían estar desactualizados (%w)", err)
	}

	fmt.Println("Operación RMUSR (modificar UID a 0) completada.")
	return nil 
}
