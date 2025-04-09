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

type MKGRP struct {
	name string
}

// ParseMkgrp analiza los tokens para el comando mkgrp
func ParseMkgrp(tokens []string) (string, error) {
	cmd := &MKGRP{}

	if len(tokens) != 1 {
		return "", errors.New("formato incorrecto. Uso: mkgrp -name=<nombre>")
	}
	// Regex simple para verificar el formato del único parámetro esperado
	re := regexp.MustCompile(`^-name=("[^"]+"|[^\s]+)$`)
	match := re.FindStringSubmatch(tokens[0])

	if match == nil {
		return "", fmt.Errorf("parámetro inválido o formato incorrecto: %s. Uso: mkgrp -name=<nombre>", tokens[0])
	}

	value := match[1] // El valor capturado (puede tener comillas)
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		value = strings.Trim(value, "\"")
	}
	if value == "" {
		return "", errors.New("el nombre del grupo (-name) no puede estar vacío")
	}
	// Validar longitud del nombre (ej: max 10 como mkusr)
	if len(value) > 10 {
		return "", fmt.Errorf("el nombre del grupo '%s' excede los 10 caracteres permitidos", value)
	}

	cmd.name = value

	// Llamar a la lógica principal del comando
	err := commandMkgrp(cmd)
	if err != nil {
		return "", err // Retornar el error de commandMkgrp
	}

	return fmt.Sprintf("MKGRP: Grupo '%s' creado correctamente.", cmd.name), nil
}

// commandMkgrp contiene la lógica principal para crear el grupo
func commandMkgrp(mkgrp *MKGRP) error {
	// 1. Verificar Autenticación y Permisos (Root)
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando mkgrp requiere inicio de sesión")
	}
	currentUser, _, partitionID := stores.Auth.GetCurrentUser()
	if currentUser != "root" {
		return fmt.Errorf("permiso denegado: solo el usuario 'root' puede ejecutar mkgrp (usuario actual: %s)", currentUser)
	}

	// 2. Obtener Partición y Superbloque
	partitionSuperblock, mountedPartition, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return fmt.Errorf("error al obtener la partición montada '%s': %w", partitionID, err)
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return errors.New("tamaño de inodo o bloque inválido en superbloque")
	}

	// 3. Encontrar y Leer Inodo/Contenido de /users.txt
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
		// Si ReadFileContent retorna "" para archivo vacío, esto está bien.
		// Si retorna error, lo manejamos.
		if oldContent != "" { // Solo retornar error si no pudimos leer nada y hubo error
			return fmt.Errorf("error leyendo el contenido de /users.txt: %w", errRead)
		}
		fmt.Println("Advertencia: /users.txt parece vacío o hubo un error menor al leer. Continuando...")
		oldContent = "" // Asegurar que sea un string vacío si hubo error menor o estaba vacío
	}
	// Asegurar que el contenido termine con un salto de línea para anexar fácilmente
	if oldContent != "" && !strings.HasSuffix(oldContent, "\n") {
		oldContent += "\n"
	}

	// 4. Parsear Contenido, Validar Grupo Existente y Obtener Nuevo GID
	fmt.Println("Validando nombre de grupo y buscando GID disponible...")
	lines := strings.Split(oldContent, "\n")
	highestGID := int32(0) // Asumimos que GID 0 no se usa, root es 1

	for _, line := range lines {
		if len(strings.TrimSpace(line)) == 0 {
			continue
		} // Ignorar líneas vacías

		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			continue
		} // Línea mal formada

		// Limpiar espacios
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}

		// Verificar si es línea de grupo y si el nombre ya existe
		if fields[1] == "G" {
			if strings.EqualFold(fields[2], mkgrp.name) {
				return fmt.Errorf("el grupo '%s' ya existe", mkgrp.name)
			}
			// Rastrear GID más alto
			gid64, errConv := strconv.ParseInt(fields[0], 10, 32)
			if errConv == nil {
				gid := int32(gid64)
				if gid > highestGID {
					highestGID = gid
				}
			}
		}
	}
	newGID := highestGID + 1
	fmt.Printf("Nuevo GID asignado: %d\n", newGID)

	// Preparar Nuevo Contenido
	newLine := fmt.Sprintf("%d,G,%s\n", newGID, mkgrp.name)
	newContent := oldContent + newLine
	newSize := int32(len(newContent))

	// Liberar Bloques Antiguos de users.txt
	fmt.Println("Liberando bloques antiguos de /users.txt...")
	errFree := structures.FreeInodeBlocks(usersInode, partitionSuperblock, partitionPath)
	if errFree != nil {
		// Es importante loguear esto pero intentamos continuar si es posible
		fmt.Printf("Error al liberar bloques antiguos de users.txt: %v. Puede haber bloques perdidos.\n", errFree)
		// return fmt.Errorf("error liberando bloques antiguos: %w", errFree) // Opcional: Fallar aquí
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
	usersInode.I_block = newAllocatedBlockIndices // Actualizar con los nuevos bloques

	usersInodeOffset := int64(partitionSuperblock.S_inode_start) + int64(usersInodeIndex)*int64(partitionSuperblock.S_inode_size)
	err = usersInode.Serialize(partitionPath, usersInodeOffset)
	if err != nil {
		return fmt.Errorf("error serializando inodo /users.txt actualizado: %w", err)
	}

	// Serializar Superbloque
	fmt.Println("Serializando SuperBlock después de MKGRP...")
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		return fmt.Errorf("error al serializar el superbloque después de mkgrp: %w", err)
	}

	return nil // Éxito
}
