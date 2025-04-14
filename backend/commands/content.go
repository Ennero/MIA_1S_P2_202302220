package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type CONTENT struct {
	id   string // ID de la partición (opcional)
	ruta string
}

func ParseContent(tokens []string) (string, error) {
	cmd := &CONTENT{}
	processedKeys := make(map[string]bool)

	rutaRegex := regexp.MustCompile(`^(?i)-ruta=(?:"([^"]+)"|([^\s"]+))$`)
	idRegex := regexp.MustCompile(`^(?i)-id=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -ruta=<directorio_interno> y opcionalmente -id=<mount_id>")
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

		// Intentar match con cada regex válida
		if match = rutaRegex.FindStringSubmatch(token); match != nil {
			key = "-ruta"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if match = idRegex.FindStringSubmatch(token); match != nil {
			key = "-id"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		}

		if !matched {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -ruta= o -id=", token)
		}
		fmt.Printf("  Match!: key='%s', value='%s'\n", key, value)

		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado: %s", key)
		}
		processedKeys[key] = true
		if value == "" {
			return "", fmt.Errorf("el valor para %s no puede estar vacío", key)
		}

		// Asignar y validar
		switch key {
		case "-ruta":
			if !strings.HasPrefix(value, "/") {
				return "", fmt.Errorf("la ruta '%s' debe ser absoluta", value)
			}
			cleanedPath := filepath.Clean(value)
			if value == "/" && cleanedPath == "." {
				cleanedPath = "/"
			}
			cmd.ruta = cleanedPath
		case "-id":
			// Aquí podríamos validar el formato del ID si quisiéramos
			cmd.id = value
		}
	}

	// Verificar obligatorio -ruta
	if !processedKeys["-ruta"] {
		return "", errors.New("falta el parámetro requerido: -ruta")
	}

	// Llamar a la lógica del comando
	contentList, err := commandContent(cmd)
	if err != nil {
		return "", err
	}

	// Formatear salida
	idMsg := cmd.id
	if idMsg == "" {
		idMsg = stores.Auth.GetPartitionID() + " (activa)"
	}
	if len(contentList) == 0 {
		return fmt.Sprintf("CONTENT: Directorio '%s' en partición '%s' está vacío.", cmd.ruta, idMsg), nil
	}
	return fmt.Sprintf("CONTENT:\n%s", strings.Join(contentList, "\n")), nil
}

func commandContent(cmd *CONTENT) ([]string, error) {
	fmt.Printf("Intentando listar contenido detallado de '%s'\n", cmd.ruta)

	// 1. Auth, Get SB, Get DiskPath... (igual que antes)
	if !stores.Auth.IsAuthenticated() {
		return nil, errors.New("content requiere login")
	}
	currentUser, userGIDStr, partitionID := stores.Auth.GetCurrentUser()
	if cmd.id != "" {
		partitionID = cmd.id
	} // Usar ID de cmd si se proporciona
	partitionSuperblock, _, diskPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return nil, fmt.Errorf("error obteniendo partición '%s': %w", partitionID, err)
	}
	if partitionSuperblock.S_magic != 0xEF53 {
		return nil, errors.New("magia SB inválida")
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return nil, errors.New("tamaño inodo/bloque inválido")
	}

	// 2. Find Target Dir Inode... (igual que antes)
	targetInodeIndex, targetInode, errFind := structures.FindInodeByPath(partitionSuperblock, diskPath, cmd.ruta)
	if errFind != nil {
		return nil, fmt.Errorf("no se encontró dir '%s': %w", cmd.ruta, errFind)
	}
	if targetInode.I_type[0] != '0' {
		return nil, fmt.Errorf("ruta '%s' no es directorio", cmd.ruta)
	}

	// 3. Check Read Permission... (igual que antes)
	if !checkPermissions(currentUser, userGIDStr, 'r', targetInode, partitionSuperblock, diskPath) {
		return nil, fmt.Errorf("permiso denegado lectura dir '%s'", cmd.ruta)
	}

	// 4. Leer Contenido, OBTENER TIPO, TAMAÑO, FECHA, PERMISOS
	fmt.Println("Leyendo entradas detalladas del directorio...")
	contentListDetailed := []string{} // Lista para guardar strings formateados
	// TODO: Implementar indirección si es necesario
	for i := 0; i < 12; i++ { // Solo directos
		blockPtr := targetInode.I_block[i]
		if blockPtr == -1 || blockPtr < 0 || blockPtr >= partitionSuperblock.S_blocks_count {
			continue
		}

		folderBlock := structures.FolderBlock{}
		blockOffset := int64(partitionSuperblock.S_block_start + blockPtr*partitionSuperblock.S_block_size)
		if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
			fmt.Printf("  Adv: Error leyendo bloque %d dir %d: %v\n", blockPtr, targetInodeIndex, err)
			continue
		}

		for j := range folderBlock.B_content {
			entry := folderBlock.B_content[j]
			if entry.B_inodo != -1 { // Entrada válida
				entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
				if entryName != "." && entryName != ".." { // Omitir . y ..

					// --- OBTENER INFO DEL INODO HIJO ---
					childInodeIndex := entry.B_inodo
					childType := byte('?')
					childSize := int32(-1)         // Tamaño -1 si no se puede leer
					childPerms := "---"            // Permisos por defecto si no se puede leer
					childMtimeStr := "Fecha desc." // Fecha por defecto

					if childInodeIndex >= 0 && childInodeIndex < partitionSuperblock.S_inodes_count {
						childInode := &structures.Inode{}
						childInodeOffset := int64(partitionSuperblock.S_inode_start + childInodeIndex*partitionSuperblock.S_inode_size)
						if err := childInode.Deserialize(diskPath, childInodeOffset); err == nil {
							// Lectura exitosa del inodo hijo
							childType = childInode.I_type[0]
							childSize = childInode.I_size
							childPerms = string(childInode.I_perm[:])
							// Formatear fecha de modificación
							mtime := time.Unix(int64(childInode.I_mtime), 0)
							childMtimeStr = mtime.Format("2006-01-02 15:04") // Formato YYYY-MM-DD HH:MM

						} else {
							fmt.Printf(" Adv: No leer inodo hijo %d ('%s'): %v\n", childInodeIndex, entryName, err)
						}
					} else {
						fmt.Printf(" Adv: Índice inodo inválido %d para '%s'.\n", childInodeIndex, entryName)
					}
					// --- FIN OBTENER INFO ---

					// Formato NUEVO: "nombre,tipo,fecha_modif,tamaño,permisos"
					formattedEntry := fmt.Sprintf("%s,%c,%s,%d,%s",
						entryName,
						childType,
						childMtimeStr,
						childSize,
						childPerms,
					)
					contentListDetailed = append(contentListDetailed, formattedEntry)
				}
			}
		}
	} // Fin for bloques

	fmt.Printf("Contenido detallado encontrado: %v\n", contentListDetailed)
	return contentListDetailed, nil // Devolver lista de strings formateados
}
