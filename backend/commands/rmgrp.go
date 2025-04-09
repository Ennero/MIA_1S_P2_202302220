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

type RMGRP struct {
	name string
}

func ParseRmgrp(tokens []string) (string, error) {
	cmd := &RMGRP{}

	if len(tokens) != 1 {
		return "", errors.New("formato incorrecto. Uso: rmgrp -name=<nombre>")
	}

	re := regexp.MustCompile(`^-name=("[^"]+"|[^\s]+)$`)
	match := re.FindStringSubmatch(tokens[0])

	if match == nil {
		return "", fmt.Errorf("parámetro inválido o formato incorrecto: %s. Uso: rmgrp -name=<nombre>", tokens[0])
	}

	value := match[1]
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		value = strings.Trim(value, "\"")
	}

	if value == "" {
		return "", errors.New("el nombre del grupo (-name) no puede estar vacío")
	}

	cmd.name = value

	err := commandRmgrp(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("RMGRP: Grupo '%s' eliminado correctamente.", cmd.name), nil
}

// commandRmgrp (Modificada la lógica de procesamiento de líneas)
func commandRmgrp(rmgrp *RMGRP) error {
	// Verificar Permisos
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando rmgrp requiere inicio de sesión")
	}
	currentUser, _, partitionID := stores.Auth.GetCurrentUser()
	if currentUser != "root" {
		return fmt.Errorf("permiso denegado: solo el usuario 'root' puede ejecutar rmgrp (usuario actual: %s)", currentUser)
	}

	// No permitir modificar el grupo "root"
	if strings.EqualFold(rmgrp.name, "root") {
		return errors.New("error: el grupo 'root' no puede ser modificado por rmgrp")
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

	// Parsear Contenido y Modificar Grupo
	fmt.Printf("Buscando grupo '%s' para modificar GID a 0...\n", rmgrp.name)
	lines := strings.Split(oldContent, "\n")
	newLines := []string{}
	foundGroup := false
	groupExistsWithGIDZero := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		fields := strings.Split(trimmedLine, ",")
		// Validar formato mínimo para línea de grupo (GID, Tipo, Nombre)
		if len(fields) < 3 {
			fmt.Printf("Advertencia: Línea con formato incorrecto en users.txt: '%s'. Se conservará.\n", line)
			newLines = append(newLines, line)
			continue
		}

		// Limpiar espacios de cada campo
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}

		isGroupLine := fields[1] == "G"                           // Verificar tipo 'G'
		isTargetGroup := strings.EqualFold(fields[2], rmgrp.name)
		currentGID := fields[0]

		// Verificar si el grupo ya tiene GID 0
		if isGroupLine && isTargetGroup && currentGID == "0" {
			fmt.Printf("Información: El grupo '%s' ya tiene GID 0.\n", rmgrp.name)
			groupExistsWithGIDZero = true     // Marcar que ya existe con GID 0
			newLines = append(newLines, line) // Mantener línea original
			foundGroup = true                 // Marcar como encontrado
			continue                          // Pasar a la siguiente línea
		}

		if isGroupLine && isTargetGroup {
			fmt.Printf("Grupo '%s' encontrado (GID: %s, Línea: '%s'). Modificando GID a 0.\n", rmgrp.name, currentGID, line)
			fields[0] = "0"                           // Cambiar GID a "0"
			modifiedLine := strings.Join(fields, ",") // Reconstruir la línea
			newLines = append(newLines, modifiedLine) // Añadir la LÍNEA MODIFICADA
			foundGroup = true
		} else {
			// No es el grupo que se busca
			newLines = append(newLines, line)
		}
	}

	// Verificar si se encontró el grupo
	if !foundGroup {
		return fmt.Errorf("error: el grupo '%s' no fue encontrado en /users.txt", rmgrp.name)
	}
	if groupExistsWithGIDZero {
		fmt.Println("No se realizaron cambios en el archivo ya que el grupo ya tenía GID 0.")
		return nil
	}

	// Preparar Nuevo Contenido Final
	newContent := strings.Join(newLines, "\n")
	// Asegurar que termine con newline si no está vacío
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newSize := int32(len(newContent))
	fmt.Printf("Nuevo contenido de users.txt preparado (%d bytes).\n", newSize)

	// Liberar Bloques Antiguos de users.txt
	fmt.Println("Liberando bloques antiguos de /users.txt...")
	errFree := structures.FreeInodeBlocks(usersInode, partitionSuperblock, partitionPath) // Asume que existe
	if errFree != nil {
		fmt.Printf("ADVERTENCIA: Error al liberar bloques antiguos de users.txt: %v. Puede haber bloques perdidos.\n", errFree)
		return fmt.Errorf("error liberando bloques antiguos: %w", errFree)
	} else {
		fmt.Println("Bloques antiguos liberados.")
	}

	// Asignar Nuevos Bloques para el nuevo contenido
	fmt.Printf("Asignando bloques para nuevo tamaño (%d bytes)...\n", newSize)
	var newAllocatedBlockIndices [15]int32
	newAllocatedBlockIndices, err = allocateDataBlocks([]byte(newContent), newSize, partitionSuperblock, partitionPath) // Asume que existe
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
	fmt.Println("Serializando SuperBlock después de RMGRP...")
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		return fmt.Errorf("ADVERTENCIA: error al serializar el superbloque después de rmgrp, los contadores podrían estar desactualizados (%w)", err)
	}

	fmt.Println("Operación RMGRP (modificar GID a 0) completada.")
	return nil 
}
