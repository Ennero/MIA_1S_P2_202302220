package commands

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	// "os" // Ya no se necesita aquí si ParseContent no valida path físico
	// "time" // No necesario

	stores "backend/stores"
	structures "backend/structures"
)

// CONTENT struct actualizada para incluir ID opcional
type CONTENT struct {
	id   string // ID de la partición (opcional, desde -id)
	ruta string // Path al directorio DENTRO DEL FS (obligatorio, desde -ruta)
}

// ParseContent: MODIFICADO para aceptar -ruta (obligatorio) y -id (opcional)
func ParseContent(tokens []string) (string, error) {
	cmd := &CONTENT{}
	processedKeys := make(map[string]bool)

	// Regex para los parámetros esperados
	rutaRegex := regexp.MustCompile(`^(?i)-ruta=(?:"([^"]+)"|([^\s"]+))$`)
	idRegex := regexp.MustCompile(`^(?i)-id=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -ruta=<directorio_interno> y opcionalmente -id=<mount_id>")
	}

	// Iterar sobre tokens
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
	} // Fin for tokens

	// Verificar obligatorio -ruta
	if !processedKeys["-ruta"] {
		return "", errors.New("falta el parámetro requerido: -ruta")
	}
	// -id es opcional, no se valida aquí

	// Llamar a la lógica del comando
	contentList, err := commandContent(cmd)
	if err != nil {
		return "", err
	}

	// Formatear salida
	idMsg := cmd.id
	if idMsg == "" {
		idMsg = stores.Auth.GetPartitionID() + " (activa)"
	} // Indicar qué partición se usó
	if len(contentList) == 0 {
		return fmt.Sprintf("CONTENT: Directorio '%s' en partición '%s' está vacío.", cmd.ruta, idMsg), nil
	}
	return fmt.Sprintf("CONTENT:\n%s", strings.Join(contentList, "\n")), nil
}

// commandContent: MODIFICADO para usar cmd.id o el de la sesión
func commandContent(cmd *CONTENT) ([]string, error) {

	var targetPartitionID string
	var diskPath string
	var partitionSuperblock *structures.SuperBlock
	var err error

	// Determinar qué partición usar (la del -id o la de la sesión)
	if cmd.id != "" {
		// Se especificó un ID, usar ese
		targetPartitionID = cmd.id
		fmt.Printf("Intentando listar contenido de '%s' en partición especificada '%s'\n", cmd.ruta, targetPartitionID)
		// Obtener SB y path para el ID especificado
		partitionSuperblock, _, diskPath, err = stores.GetMountedPartitionSuperblock(targetPartitionID)
		if err != nil {
			// Error si el ID especificado no está montado o no se puede leer
			return nil, fmt.Errorf("error obteniendo partición especificada con id '%s': %w", targetPartitionID, err)
		}
	} else {
		// No se especificó ID, usar la partición de la sesión actual
		fmt.Printf("Intentando listar contenido de '%s' en partición activa\n", cmd.ruta)
		if !stores.Auth.IsAuthenticated() {
			return nil, errors.New("comando content requiere inicio de sesión si no se especifica -id")
		}
		_, _, targetPartitionID = stores.Auth.GetCurrentUser() // Obtener ID de sesión
		if targetPartitionID == "" {
			return nil, errors.New("no hay partición activa en la sesión actual y no se especificó -id")
		}
		fmt.Printf("  Usando partición activa: %s\n", targetPartitionID)
		partitionSuperblock, _, diskPath, err = stores.GetMountedPartitionSuperblock(targetPartitionID)
		if err != nil {
			return nil, fmt.Errorf("error obteniendo partición activa '%s': %w", targetPartitionID, err)
		}
	}

	// Validar SB (común para ambos casos)
	if partitionSuperblock.S_magic != 0xEF53 {
		return nil, fmt.Errorf("magia de superbloque inválida en partición '%s'", targetPartitionID)
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return nil, fmt.Errorf("tamaño de inodo/bloque inválido en '%s'", targetPartitionID)
	}

	// --- A partir de aquí, la lógica es la misma, usando el SB y diskPath obtenidos ---

	// Encontrar Inodo del Directorio (-ruta)
	fmt.Printf("Buscando inodo para directorio interno: %s (en disco %s)\n", cmd.ruta, diskPath)
	targetInodeIndex, targetInode, errFind := structures.FindInodeByPath(partitionSuperblock, diskPath, cmd.ruta)
	if errFind != nil {
		return nil, fmt.Errorf("error: no se encontró el directorio '%s' en partición '%s': %w", cmd.ruta, targetPartitionID, errFind)
	}

	// Verificar que es un Directorio
	if targetInode.I_type[0] != '0' {
		return nil, fmt.Errorf("error: la ruta '%s' no es un directorio (tipo %c)", cmd.ruta, targetInode.I_type[0])
	}
	fmt.Printf("Directorio encontrado (inodo %d)\n", targetInodeIndex)

	// Verificar Permiso de Lectura (usando el usuario de la sesión actual)
	currentUser, userGIDStr, _ := stores.Auth.GetCurrentUser()   // Necesitamos el usuario actual para permisos
	if !stores.Auth.IsAuthenticated() && currentUser != "root" { // Doble chequeo si no hay sesión y no es root implícito
		return nil, errors.New("se requiere sesión para verificar permisos (usuario no root)")
	}
	fmt.Printf("Verificando permiso de lectura para usuario '%s' en '%s'...\n", currentUser, cmd.ruta)
	if !checkPermissions(currentUser, userGIDStr, 'r', targetInode, partitionSuperblock, diskPath) { // Asume checkPermissions existe
		return nil, fmt.Errorf("permiso denegado: usuario '%s' no puede leer '%s'", currentUser, cmd.ruta)
	}
	fmt.Println("Permiso de lectura concedido.")

	// Leer Contenido del Directorio
	fmt.Println("Leyendo entradas del directorio...")
	contentListWithType := []string{}
	// TODO: Implementar indirección
	for i := 0; i < 12; i++ { // Solo directos
		blockPtr := targetInode.I_block[i]
		if blockPtr == -1 || blockPtr < 0 || blockPtr >= partitionSuperblock.S_blocks_count {
			continue
		}

		folderBlock := structures.FolderBlock{}
		blockOffset := int64(partitionSuperblock.S_block_start + blockPtr*partitionSuperblock.S_block_size)
		if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
			fmt.Printf("  Advertencia: Error leyendo bloque %d dir %d: %v.\n", blockPtr, targetInodeIndex, err)
			continue
		}

		for j := range folderBlock.B_content {
			entry := folderBlock.B_content[j]
			if entry.B_inodo != -1 {
				entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
				if entryName != "." && entryName != ".." { // Omitir . y ..
					childInodeIndex := entry.B_inodo
					childType := byte('?')
					if childInodeIndex >= 0 && childInodeIndex < partitionSuperblock.S_inodes_count {
						childInode := &structures.Inode{}
						childInodeOffset := int64(partitionSuperblock.S_inode_start + childInodeIndex*partitionSuperblock.S_inode_size)
						if err := childInode.Deserialize(diskPath, childInodeOffset); err == nil {
							childType = childInode.I_type[0]
						} else {
							fmt.Printf(" Adv: No se pudo leer inodo hijo %d ('%s') tipo: %v\n", childInodeIndex, entryName, err)
						}
					} else {
						fmt.Printf(" Adv: Índice inodo inválido %d para '%s'.\n", childInodeIndex, entryName)
					}
					formattedEntry := fmt.Sprintf("%s,%c", entryName, childType)
					contentListWithType = append(contentListWithType, formattedEntry)
				}
			}
		}
	}

	fmt.Printf("Contenido encontrado (nombre,tipo): %v\n", contentListWithType)
	return contentListWithType, nil
}

// --- Faltan checkPermissions y otras helpers ---
