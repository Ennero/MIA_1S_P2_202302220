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

type CHGRP struct {
	user string 
	grp  string 
}

func ParseChgrp(tokens []string) (string, error) {
	cmd := &CHGRP{}
	expectedArgs := map[string]bool{"-user": false, "-grp": false}

	args := strings.Join(tokens, " ")
	re := regexp.MustCompile(`-(user|grp)=("[^"]+"|[^\s]+)`)
	matches := re.FindAllStringSubmatch(args, -1)

	if len(matches) != 2 {
		return "", fmt.Errorf("número incorrecto de parámetros. Se esperan -user y -grp. Encontrados: %d", len(matches))
	}

	for _, match := range matches {
		key := strings.ToLower(match[1])
		value := match[2]
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
		case "grp":
			cmd.grp = value
			expectedArgs["-grp"] = true
		default:
			return "", fmt.Errorf("parámetro desconocido detectado: %s", key)
		}
	}
	if !expectedArgs["-user"] || !expectedArgs["-grp"] {
		return "", errors.New("faltan parámetros obligatorios: se requieren -user y -grp")
	}

	err := commandChgrp(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("CHGRP: Grupo del usuario '%s' cambiado a '%s'.", cmd.user, cmd.grp), nil
}

func commandChgrp(chgrp *CHGRP) error {
	// Verificar Permisos 
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando chgrp requiere inicio de sesión")
	}
	currentUser, _, partitionID := stores.Auth.GetCurrentUser()
	if currentUser != "root" {
		return fmt.Errorf("permiso denegado: solo 'root' puede ejecutar chgrp (actual: %s)", currentUser)
	}

	// Obtener Partición y Superbloque
	partitionSuperblock, mountedPartition, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return fmt.Errorf("error al obtener partición montada '%s': %w", partitionID, err)
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return errors.New("tamaño inválido de inodo/bloque en superbloque")
	}

	//Encontrar y Leer /users.txt
	fmt.Println("Buscando inodo para /users.txt...")
	usersInodeIndex, usersInode, errFind := structures.FindInodeByPath(partitionSuperblock, partitionPath, "/users.txt")
	if errFind != nil {
		return fmt.Errorf("error crítico: no se pudo encontrar /users.txt: %w", errFind)
	}
	if usersInode.I_type[0] != '1' {
		return errors.New("error crítico: /users.txt no es un archivo")
	}

	fmt.Println("Leyendo contenido actual de /users.txt...")
	oldContent, errRead := structures.ReadFileContent(partitionSuperblock, partitionPath, usersInode)
	if errRead != nil && oldContent == "" {
		return fmt.Errorf("error leyendo /users.txt: %w", errRead)
	}
	if oldContent != "" && !strings.HasSuffix(oldContent, "\n") {
		oldContent += "\n"
	}

	// Parsear Contenido y Validaciones
	fmt.Printf("Validando usuario '%s' y nuevo grupo '%s'...\n", chgrp.user, chgrp.grp)
	lines := strings.Split(oldContent, "\n")
	newLines := make([]string, 0, len(lines)) // Slice para reconstruir el archivo
	userFound := false
	groupFound := false
	userLineModified := false // bandera para saber si modificamos la línea del usuario

	// Validar que el nuevo grupo exista
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}
		fields := strings.Split(trimmedLine, ",")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		if len(fields) == 3 && fields[1] == "G" && strings.EqualFold(fields[2], chgrp.grp) {
			groupFound = true
			break
		}
	}
	if !groupFound {
		return fmt.Errorf("error: el nuevo grupo '%s' no existe", chgrp.grp)
	}
	fmt.Printf("Grupo '%s' encontrado y válido.\n", chgrp.grp)

	// Encontrar usuario y reconstruir archivo
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		fields := strings.Split(trimmedLine, ",")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}

		// Verificar si es la línea del usuario a modificar
		if len(fields) == 5 && fields[1] == "U" && strings.EqualFold(fields[3], chgrp.user) {
			userFound = true
			// Modificar la línea cambiando el nombre del grupo
			modifiedLine := fmt.Sprintf("%s,U,%s,%s,%s", fields[0], chgrp.grp, fields[3], fields[4])
			newLines = append(newLines, modifiedLine)
			userLineModified = true
			fmt.Printf("Línea del usuario '%s' modificada a: %s\n", chgrp.user, modifiedLine)
		} else {
			// Conservar la línea original
			newLines = append(newLines, line)
		}
	}

	// Valida si se encontró al usuario
	if !userFound {
		return fmt.Errorf("error: el usuario '%s' no fue encontrado", chgrp.user)
	}
	if !userLineModified {
		return errors.New("error interno: se encontró el usuario pero no se modificó la línea")
	}

	// Preparar Nuevo Contenido Final
	newContent := strings.Join(newLines, "\n")
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newSize := int32(len(newContent))
	fmt.Printf("Nuevo contenido de users.txt preparado (%d bytes).\n", newSize)

	// Libera Bloques Antiguos de users.txt
	fmt.Println("Liberando bloques antiguos de /users.txt...")
	errFree := structures.FreeInodeBlocks(usersInode, partitionSuperblock, partitionPath)
	if errFree != nil {
		fmt.Printf("ADVERTENCIA: Error al liberar bloques: %v\n", errFree)
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
	fmt.Println("Serializando SuperBlock después de CHGRP...")
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		return fmt.Errorf("error al serializar el superbloque después de chgrp: %w", err)
	}

	return nil // Éxito
}
