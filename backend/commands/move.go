package commands

import (
	"errors"
	"fmt"
	"path/filepath" // Para Dir/Base
	"regexp"
	"strings"
	"time" // Para timestamps

	stores "backend/stores"
	structures "backend/structures"
)

// Move struct (como la tenías)
type Move struct {
	path    string // Path actual absoluto del archivo/carpeta origen
	destino string // Path absoluto del directorio destino
}

func ParseMove(tokens []string) (string, error) {
	cmd := &Move{}
	processedKeys := make(map[string]bool)

	pathRegex := regexp.MustCompile(`^(?i)-path=(?:"([^"]+)"|([^\s"]+))$`)
	destinoRegex := regexp.MustCompile(`^(?i)-destino=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -path y -destino")
	}

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		fmt.Printf("Procesando token: '%s'\n", token)

		var match []string
		var key string
		var value string
		matched := false

		// Intentar match con cada regex
		if match = pathRegex.FindStringSubmatch(token); match != nil {
			key = "-path"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			} // Extraer valor
			matched = true
		} else if match = destinoRegex.FindStringSubmatch(token); match != nil {
			key = "-destino"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			} // Extraer valor
			matched = true
		}

		// Error si no coincide con ninguno
		if !matched {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -path= o -destino=", token)
		}

		fmt.Printf("  Match!: key='%s', value='%s'\n", key, value)

		// Validar duplicados
		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado: %s", key)
		}
		processedKeys[key] = true
		if value == "" {
			return "", fmt.Errorf("el valor para %s no puede estar vacío", key)
		}

		// Asignar y validar
		switch key {
		case "-path":
			if !strings.HasPrefix(value, "/") {
				return "", fmt.Errorf("el path '%s' debe ser absoluto", value)
			}
			if value == "/" {
				return "", errors.New("error: no se puede mover el directorio raíz '/'")
			}
			cmd.path = value
		case "-destino":
			if !strings.HasPrefix(value, "/") {
				return "", fmt.Errorf("el path de destino '%s' debe ser absoluto", value)
			}
			cmd.destino = value
		}
	} 

	// Verificar obligatorios
	if !processedKeys["-path"] {
		return "", errors.New("falta el parámetro requerido: -path")
	}
	if !processedKeys["-destino"] {
		return "", errors.New("falta el parámetro requerido: -destino")
	}

	// Verificar que origen y destino no sean el mismo o inválidos
	if cmd.path == cmd.destino {
		return "", errors.New("error: el origen y el destino no pueden ser iguales")
	}
	// Verificar si destino está dentro de origen
	if strings.HasPrefix(cmd.destino, cmd.path+"/") {
		return "", fmt.Errorf("error: el destino '%s' no puede estar dentro del origen '%s'", cmd.destino, cmd.path)
	}

	err := commandMove(cmd) 
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MOVE: '%s' movido a '%s' correctamente.", cmd.path, cmd.destino), nil
}


// commandMove: Lógica para mover archivo/carpeta
func commandMove(cmd *Move) error {
	fmt.Printf("Intentando mover '%s' a '%s'\n", cmd.path, cmd.destino)

	// Autenticación y obtener SB/Partición
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando move requiere inicio de sesión")
	}
	currentUser, userGIDStr, partitionID := stores.Auth.GetCurrentUser()
	partitionSuperblock, _, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return fmt.Errorf("error obteniendo partición montada '%s': %w", partitionID, err)
	}
	if partitionSuperblock.S_magic != 0xEF53 {
		return errors.New("magia de superbloque inválida")
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return errors.New("tamaño de inodo o bloque inválido")
	}

	// Validar Origen (-path)
	fmt.Printf("Validando origen: %s\n", cmd.path)
	sourceInodeIndex, sourceInode, errFindSource := structures.FindInodeByPath(partitionSuperblock, partitionPath, cmd.path)
	if errFindSource != nil {
		return fmt.Errorf("error: no se encontró el origen '%s': %w", cmd.path, errFindSource)
	}
	fmt.Printf("Inodo origen encontrado: %d (Tipo: %c)\n", sourceInodeIndex, sourceInode.I_type[0])
	// Prohibir mover archivos/carpetas esenciales
	if cmd.path == "/users.txt" || (cmd.path == "/.journal" && partitionSuperblock.S_filesystem_type == 3) {
		return fmt.Errorf("error: el archivo/directorio '%s' no puede ser movido", cmd.path)
	}

	// Validar Destino (-destino)
	fmt.Printf("Validando destino: %s\n", cmd.destino)
	destDirInodeIndex, destDirInode, errFindDest := structures.FindInodeByPath(partitionSuperblock, partitionPath, cmd.destino)
	if errFindDest != nil {
		return fmt.Errorf("error: no se encontró directorio destino '%s': %w", cmd.destino, errFindDest)
	}
	if destDirInode.I_type[0] != '0' {
		return fmt.Errorf("error: destino '%s' no es directorio", cmd.destino)
	}
	fmt.Printf("Inodo destino encontrado: %d\n", destDirInodeIndex)

	// Validar Padre Origen
	sourceParentPath := filepath.Dir(cmd.path)
	if sourceParentPath == "." {
		sourceParentPath = "/"
	}
	fmt.Printf("Buscando padre origen: %s\n", sourceParentPath)
	sourceParentInodeIndex, sourceParentInode, errFindSourceParent := structures.FindInodeByPath(partitionSuperblock, partitionPath, sourceParentPath)
	if errFindSourceParent != nil {
		return fmt.Errorf("error crítico: no se encontró padre origen '%s': %w", sourceParentPath, errFindSourceParent)
	}
	if sourceParentInode.I_type[0] != '0' {
		return fmt.Errorf("error crítico: padre origen '%s' no es directorio", sourceParentPath)
	}
	fmt.Printf("Padre origen encontrado: %d\n", sourceParentInodeIndex)

	// Evitar mover un directorio a sí mismo (ya cubierto por la validación de path vs destino?)
	if sourceInodeIndex == destDirInodeIndex {
		return errors.New("error: no se puede mover un directorio dentro de sí mismo (origen y destino apuntan al mismo inodo de directorio)")
	}

	// Verificar Permisos
	fmt.Printf("Verificando permisos para usuario '%s'...\n", currentUser)
	if currentUser != "root" {
		// Permiso escritura en Origen 
		if !checkPermissions(currentUser, userGIDStr, 'w', sourceInode, partitionSuperblock, partitionPath) {
			return fmt.Errorf("permiso denegado: escritura sobre origen '%s'", cmd.path)
		}
		// Permiso escritura en Padre Destino
		if !checkPermissions(currentUser, userGIDStr, 'w', destDirInode, partitionSuperblock, partitionPath) {
			return fmt.Errorf("permiso denegado: escritura sobre directorio destino '%s'", cmd.destino)
		}
		// Permiso escritura en Padre Origen
		if !checkPermissions(currentUser, userGIDStr, 'w', sourceParentInode, partitionSuperblock, partitionPath) {
			return fmt.Errorf("permiso denegado: escritura sobre directorio padre origen '%s'", sourceParentPath)
		}
	}
	fmt.Println("Permisos concedidos.")

	// Verificar si el nombre ORIGINAL ya existe en el NUEVO destino
	sourceBaseName := filepath.Base(cmd.path) // El nombre que se moverá
	fmt.Printf("Verificando si '%s' ya existe en destino '%s' (inodo %d)...\n", sourceBaseName, cmd.destino, destDirInodeIndex)
	exists, _, _ := findEntryInParent(destDirInode, sourceBaseName, partitionSuperblock, partitionPath)
	if exists {
		return fmt.Errorf("error: '%s' ya existe en el directorio destino '%s'", sourceBaseName, cmd.destino)
	}
	fmt.Printf("Nombre '%s' disponible en destino.\n", sourceBaseName)

	//  INICIO DE OPERACIONES DE MOVIMIENTO

	// Añadir Entrada en Directorio Destino 
	fmt.Printf("Añadiendo entrada '%s' -> %d en directorio destino %d...\n", sourceBaseName, sourceInodeIndex, destDirInodeIndex)
	errAdd := addEntryToParent(destDirInodeIndex, sourceBaseName, sourceInodeIndex, partitionSuperblock, partitionPath)
	if errAdd != nil {
		return fmt.Errorf("error añadiendo entrada a destino '%s': %w", cmd.destino, errAdd)
	}
	fmt.Println("Entrada añadida a directorio destino.")

	// Eliminar Entrada del Directorio Padre Origen
	fmt.Printf("Eliminando entrada '%s' de directorio padre origen %d...\n", sourceBaseName, sourceParentInodeIndex)
	entryRemoved := false
	for i := 0; i < 12; i++ { // Solo directos
		blockPtr := sourceParentInode.I_block[i]
		if blockPtr == -1 || blockPtr < 0 || blockPtr >= partitionSuperblock.S_blocks_count {
			continue
		}

		folderBlock := structures.FolderBlock{}
		blockOffset := int64(partitionSuperblock.S_block_start + blockPtr*partitionSuperblock.S_block_size)
		if err := folderBlock.Deserialize(partitionPath, blockOffset); err != nil {
			fmt.Printf("Advertencia: Error leyendo bloque %d padre origen: %v\n", blockPtr, err)
			continue
		}

		blockNeedsWrite := false
		for j := range folderBlock.B_content {
			// Buscar la entrada por el índice de inodo
			if folderBlock.B_content[j].B_inodo == sourceInodeIndex {
				currentName := strings.TrimRight(string(folderBlock.B_content[j].B_name[:]), "\x00")
				if currentName == sourceBaseName {
					fmt.Printf("  Entrada encontrada en bloque %d, índice %d. Eliminando...\n", blockPtr, j)
					folderBlock.B_content[j].B_inodo = -1
					folderBlock.B_content[j].B_name = [12]byte{}
					entryRemoved = true
					blockNeedsWrite = true
					break
				}
			}
		}
		if blockNeedsWrite {
			if err := folderBlock.Serialize(partitionPath, blockOffset); err != nil {
				return fmt.Errorf("¡ERROR INCONSISTENCIA! Guardando bloque padre origen %d modificado: %w", blockPtr, err)
			}
			break
		}
	}
	if !entryRemoved {
		return fmt.Errorf("¡ERROR INCONSISTENCIA! No se encontró entrada original '%s' en padre origen %d", sourceBaseName, sourceParentInodeIndex)
	}
	fmt.Println("Entrada eliminada de directorio padre origen.")

	// Actualizar ".." en Inodo Movido (SI ES DIRECTORIO)
	if sourceInode.I_type[0] == '0' {
		fmt.Printf("Actualizando '..' en directorio movido (inodo %d) para apuntar a nuevo padre %d...\n", sourceInodeIndex, destDirInodeIndex)
		// Asumir que está en el primer bloque directo
		firstBlockPtr := sourceInode.I_block[0]
		if firstBlockPtr != -1 && firstBlockPtr >= 0 && firstBlockPtr < partitionSuperblock.S_blocks_count {
			folderBlock := structures.FolderBlock{}
			blockOffset := int64(partitionSuperblock.S_block_start + firstBlockPtr*partitionSuperblock.S_block_size)
			// Leer el bloque del directorio movido
			if err := folderBlock.Deserialize(partitionPath, blockOffset); err == nil {
				// Verificar que la entrada '..' esté donde se espera (índice 1)
				if strings.TrimRight(string(folderBlock.B_content[1].B_name[:]), "\x00") == ".." {
					folderBlock.B_content[1].B_inodo = destDirInodeIndex // Apuntar al nuevo padre
					// Reescribir el bloque modificado
					if err := folderBlock.Serialize(partitionPath, blockOffset); err != nil {
						fmt.Printf("Advertencia: Error guardando bloque %d con '..' actualizado: %v\n", firstBlockPtr, err)
					} else {
						fmt.Println("  Entrada '..' actualizada.")
					}
				} else {
					fmt.Printf("Advertencia: No se encontró '..' en posición esperada bloque %d.\n", firstBlockPtr)
				}
			} else {
				fmt.Printf("Advertencia: Error leyendo bloque %d para actualizar '..': %v\n", firstBlockPtr, err)
			}
		} else {
			fmt.Printf("Advertencia: Primer bloque del dir movido %d es inválido. No se pudo actualizar '..'.\n", sourceInodeIndex)
		}
	}

	// Actualizar tiempos
	fmt.Println("Actualizando timestamps...")
	now := float32(time.Now().Unix())

	// Padre Origen 
	sourceParentInode.I_mtime = now
	sourceParentInode.I_atime = now
	sourceParentInodeOffset := int64(partitionSuperblock.S_inode_start + sourceParentInodeIndex*partitionSuperblock.S_inode_size)
	if err := sourceParentInode.Serialize(partitionPath, sourceParentInodeOffset); err != nil {
		fmt.Printf("Advertencia: Error guardando inodo padre origen %d: %v\n", sourceParentInodeIndex, err)
	}

	// Padre Destino 
	destDirInode.I_mtime = now
	destDirInode.I_atime = now
	destDirInodeOffset := int64(partitionSuperblock.S_inode_start + destDirInodeIndex*partitionSuperblock.S_inode_size)
	if err := destDirInode.Serialize(partitionPath, destDirInodeOffset); err != nil {
		fmt.Printf("Advertencia: Error guardando inodo padre destino %d: %v\n", destDirInodeIndex, err)
	}

	// Objetivo
	sourceInode.I_ctime = now // Hora de cambio del inodo
	sourceInode.I_atime = now // Accedimos para leerlo/moverlo
	sourceInodeOffset := int64(partitionSuperblock.S_inode_start + sourceInodeIndex*partitionSuperblock.S_inode_size)
	if err := sourceInode.Serialize(partitionPath, sourceInodeOffset); err != nil {
		return fmt.Errorf("error crítico al guardar inodo objetivo %d: %w", sourceInodeIndex, err)
	}

	fmt.Println("MOVE completado exitosamente.")
	return nil
}
