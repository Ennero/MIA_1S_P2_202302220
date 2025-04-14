package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	utils "backend/utils"
	"errors"
	"fmt"
	"path/filepath" // Para obtener Dir/Base
	"regexp"
	"strings"
	"time"
)

type REMOVE struct {
	path string
}

func ParseRemove(tokens []string) (string, error) {
	cmd := &REMOVE{}
	processedKeys := make(map[string]bool)

	pathRegex := regexp.MustCompile(`^(?i)-path=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -path=<valor>")
	}

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		match := pathRegex.FindStringSubmatch(token)
		if match != nil {
			key := "-path"
			value := ""
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			} // Extraer valor

			if processedKeys[key] {
				return "", fmt.Errorf("parámetro duplicado: %s", key)
			}
			processedKeys[key] = true

			if value == "" {
				return "", errors.New("el valor para -path no puede estar vacío")
			}

			if !strings.HasPrefix(value, "/") {
				return "", fmt.Errorf("el path '%s' debe ser absoluto (empezar con /)", value)
			}
			if value == "/" {
				return "", errors.New("error: no se puede eliminar el directorio raíz '/'")
			}

			cmd.path = value

		} else {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -path=<valor>", token)
		}
	}

	if !processedKeys["-path"] {
		return "", errors.New("falta el parámetro requerido: -path")
	}

	err := commandRemove(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("REMOVE: '%s' eliminado correctamente.", cmd.path), nil
}

func commandRemove(cmd *REMOVE) error {
	fmt.Printf("Intentando eliminar: %s\n", cmd.path)

	// Verificar Autenticación
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando remove requiere inicio de sesión")
	}
	currentUser, userGIDStr, partitionID := stores.Auth.GetCurrentUser()

	// Obtener Superbloque, INFO DE PARTICIÓN y path del disco
	partitionSuperblock, mountedPartition, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return fmt.Errorf("error obteniendo partición montada '%s': %w", partitionID, err)
	}
	if partitionSuperblock.S_magic != 0xEF53 {
		return errors.New("magia de superbloque inválida")
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return errors.New("tamaño de inodo o bloque inválido")
	}

	// Encontrar Inodo Objetivo
	targetInodeIndex, _, errFind := structures.FindInodeByPath(partitionSuperblock, partitionPath, cmd.path)
	if errFind != nil {
		return fmt.Errorf("error: no se encontró '%s': %w", cmd.path, errFind)
	}

	// Encontrar Inodo Padre
	parentPath := filepath.Dir(cmd.path)
	if parentPath == "." {
		parentPath = "/"
	}
	parentInodeIndex, parentInode, errFindParent := structures.FindInodeByPath(partitionSuperblock, partitionPath, parentPath)
	if errFindParent != nil {
		return fmt.Errorf("error crítico: no se encontró padre '%s': %w", parentPath, errFindParent)
	}
	if parentInode.I_type[0] != '0' {
		return fmt.Errorf("error crítico: padre '%s' no es directorio", parentPath)
	}

	// Verificar Permiso de Escritura en el PADRE
	if currentUser != "root" {
		ownerPerm := parentInode.I_perm[1] // Asumiendo formato tipo '6' o '7' o 'w'
		canWriteParent := ownerPerm == 'w' || ownerPerm == 'W' || ownerPerm == '6' || ownerPerm == '7'
		isOwner := true
		if !isOwner || !canWriteParent {
			return fmt.Errorf("permiso denegado: no tienes permiso de escritura en '%s'", parentPath)
		}
	}

	// Llamar a la Función Recursiva de Borrado
	fmt.Printf("Iniciando eliminación recursiva desde inodo %d...\n", targetInodeIndex)
	errRemove := recursiveRemove(targetInodeIndex, partitionSuperblock, partitionPath, currentUser, userGIDStr)
	if errRemove != nil {
		return fmt.Errorf("error durante la eliminación: %w", errRemove)
	}

	// Eliminar Entrada del Directorio Padre
	fmt.Printf("Eliminando entrada '%s' del directorio padre (inodo %d)...\n", filepath.Base(cmd.path), parentInodeIndex)
	entryName := filepath.Base(cmd.path)
	entryRemoved := false
	parentModified := false

	for i := 0; i < 12; i++ { // Solo directos
		blockPtr := parentInode.I_block[i]
		if blockPtr == -1 {
			continue
		}
		if blockPtr < 0 || blockPtr >= partitionSuperblock.S_blocks_count {
			continue
		}

		folderBlock := structures.FolderBlock{}
		blockOffset := int64(partitionSuperblock.S_block_start + blockPtr*partitionSuperblock.S_block_size)
		if err := folderBlock.Deserialize(partitionPath, blockOffset); err != nil {
			fmt.Printf("Advertencia: Error leyendo bloque %d padre: %v\n", blockPtr, err)
			continue
		}

		blockWasModified := false
		for j := range folderBlock.B_content {
			if folderBlock.B_content[j].B_inodo == targetInodeIndex {
				name := strings.TrimRight(string(folderBlock.B_content[j].B_name[:]), "\x00")
				if name == entryName {
					fmt.Printf("  Encontrada entrada en bloque %d, índice %d. Eliminando...\n", blockPtr, j)
					folderBlock.B_content[j].B_inodo = -1
					folderBlock.B_content[j].B_name = [12]byte{}
					entryRemoved = true
					blockWasModified = true
					break
				}
			}
		}

		if blockWasModified {
			if err := folderBlock.Serialize(partitionPath, blockOffset); err != nil {
				return fmt.Errorf("error crítico: guardando bloque padre %d modificado: %w", blockPtr, err)
			}
			parentModified = true
			break
		}
	}

	if !entryRemoved {
		fmt.Printf("Advertencia: No se encontró entrada '%s' en bloques directos padre %d.\n", entryName, parentInodeIndex)
	}

	// Actualizar Tiempos del Padre
	if parentModified {
		fmt.Println("Actualizando mtime/atime del inodo padre...")
		parentInode.I_mtime = float32(time.Now().Unix())
		parentInode.I_atime = parentInode.I_mtime
		parentInodeOffset := int64(partitionSuperblock.S_inode_start + parentInodeIndex*partitionSuperblock.S_inode_size)
		if err := parentInode.Serialize(partitionPath, parentInodeOffset); err != nil {
			fmt.Printf("Advertencia: Error guardando inodo padre %d actualizado: %v\n", parentInodeIndex, err)
		}
	}

	// Serializar Superbloque Final
	fmt.Println("Serializando SuperBlock después de REMOVE...")
	// Usar la variable 'mountedPartition' obtenida al principio
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		return fmt.Errorf("ADVERTENCIA: error al serializar superbloque después de remove: %w", err)
	}

	if partitionSuperblock.S_filesystem_type == 3 {
		journalEntryData := structures.Information{
			I_operation: utils.StringToBytes10("remove"),
			I_path:      utils.StringToBytes32(cmd.path), // Path original a borrar
			I_content:   utils.StringToBytes64(""),       // Contenido vacío
		}
		errJournal := utils.AppendToJournal(journalEntryData, partitionSuperblock, partitionPath)
		if errJournal != nil {
			fmt.Printf("Advertencia: Falla al escribir en journal para remove '%s': %v\n", cmd.path, errJournal)
		}
	}

	fmt.Println("REMOVE completado.")
	return nil
}

// Elimina un inodo y su contenido recursivamente.
func recursiveRemove(inodeIndex int32, sb *structures.SuperBlock, diskPath string, currentUser string, userGIDStr string) error {
	fmt.Printf("--> recursiveRemove: Procesando inodo %d\n", inodeIndex)

	// Validar índice antes de usar
	if inodeIndex < 0 || inodeIndex >= sb.S_inodes_count {
		return fmt.Errorf("índice de inodo inválido %d", inodeIndex)
	}

	// Leer Inodo
	inode := &structures.Inode{}
	inodeOffset := int64(sb.S_inode_start + inodeIndex*sb.S_inode_size)
	if err := inode.Deserialize(diskPath, inodeOffset); err != nil {
		return fmt.Errorf("no se pudo leer inodo %d para eliminar: %w", inodeIndex, err)
	}

	// Verificar Permiso de Escritura sobre ESTE inodo/directorio
	fmt.Printf("    Verificando permisos para usuario '%s' en inodo %d (Perms: %s)...\n", currentUser, inodeIndex, string(inode.I_perm[:]))
	canWrite := false
	if currentUser == "root" {
		canWrite = true
	} else {
		ownerPerm := inode.I_perm[0] // Permiso del dueño
		if ownerPerm == 'w' || ownerPerm == 'W' || ownerPerm == '6' || ownerPerm == '7' {
			canWrite = true
		}
	}

	if !canWrite {
		return fmt.Errorf("permiso denegado para eliminar inodo %d", inodeIndex)
	}
	fmt.Println("    Permiso concedido.")

	// Procesar según tipo
	if inode.I_type[0] == '1' {
		fmt.Printf("    Inodo %d es un ARCHIVO. Liberando bloques...\n", inodeIndex)
		err := structures.FreeInodeBlocks(inode, sb, diskPath)
		if err != nil {
			fmt.Printf("    Advertencia: Error liberando bloques para inodo %d: %v\n", inodeIndex, err)
		} else {
			fmt.Printf("    Bloques de datos para inodo %d liberados.\n", inodeIndex)
		}

	} else if inode.I_type[0] == '0' {
		fmt.Printf("    Inodo %d es un DIRECTORIO. Procesando contenido recursivamente...\n", inodeIndex)

		// Iterar bloques directos del directorio
		for i := 0; i < 12; i++ {
			blockPtr := inode.I_block[i]
			if blockPtr == -1 {
				continue
			}
			if blockPtr < 0 || blockPtr >= sb.S_blocks_count {
				fmt.Printf("    Advertencia: Puntero inválido %d en dir %d. Saltando bloque.\n", blockPtr, inodeIndex)
				continue
			}

			fmt.Printf("      Procesando bloque de directorio %d...\n", blockPtr)
			folderBlock := structures.FolderBlock{}
			blockOffset := int64(sb.S_block_start + blockPtr*sb.S_block_size)
			if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
				fmt.Printf("      Advertencia: Error leyendo bloque %d del dir %d: %v. Saltando.\n", blockPtr, inodeIndex, err)
				continue // Saltar bloque si no se puede leer
			}

			// Iterar entradas DENTRO del bloque
			for j := range folderBlock.B_content {
				entry := folderBlock.B_content[j]
				if entry.B_inodo == -1 {
					continue
				} // Slot libre

				entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")

				// Saltar "." y ".."
				if entryName == "." || entryName == ".." {
					continue
				}

				fmt.Printf("        Llamando recursiveRemove para '%s' (inodo %d)...\n", entryName, entry.B_inodo)
				// Llamada RECURSIVA para el hijo
				errRec := recursiveRemove(entry.B_inodo, sb, diskPath, currentUser, userGIDStr)
				if errRec != nil {
					// Si falla borrar un hijo, ABORTAR y retornar el error
					fmt.Printf("        ¡FALLÓ la eliminación recursiva de '%s' (inodo %d)! Abortando.\n", entryName, entry.B_inodo)
					return fmt.Errorf("no se pudo eliminar '%s' dentro de inodo %d: %w", entryName, inodeIndex, errRec)
				}
				fmt.Printf("        Eliminación recursiva de '%s' (inodo %d) completada.\n", entryName, entry.B_inodo)
			}
			// liberar ESTE bloque de directorio.
			fmt.Printf("      Liberando bloque de directorio %d...\n", blockPtr)
			if err := sb.UpdateBitmapBlock(diskPath, blockPtr, '0'); err != nil { // <-- Añadir '0'
				fmt.Printf("      ¡ERROR GRAVE! No se pudo liberar bloque %d en bitmap: %v\n", blockPtr, err)
				return fmt.Errorf("error crítico liberando bloque %d en bitmap: %w", blockPtr, err)
			}
			sb.S_free_blocks_count++

		}

	} else {
		fmt.Printf("    Advertencia: Inodo %d tiene tipo desconocido '%c'. Intentando liberar de todas formas.\n", inodeIndex, inode.I_type[0])
	}

	// Liberar el Inodo Actual después de procesar hijos y liberar bloques de datos/directorio
	fmt.Printf("    Liberando inodo %d...\n", inodeIndex)
	if err := sb.UpdateBitmapInode(diskPath, inodeIndex, '0'); err != nil { // <-- Añadir '0'
		return fmt.Errorf("error crítico liberando inodo %d en bitmap: %w", inodeIndex, err)
	}
	sb.S_free_inodes_count++

	fmt.Printf("<-- recursiveRemove: Inodo %d procesado.\n", inodeIndex)
	return nil // Éxito para este inodo
}
