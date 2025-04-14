package commands

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	stores "backend/stores"
	structures "backend/structures"
	utils "backend/utils"
)

type CHOWN struct {
	path      string
	usuario   string
	recursive bool
}

func ParseChown(tokens []string) (string, error) {
	cmd := &CHOWN{}
	processedKeys := make(map[string]bool)

	// Regex para los parámetros esperados
	pathRegex := regexp.MustCompile(`^(?i)-path=(?:"([^"]+)"|([^\s"]+))$`)
	usuarioRegex := regexp.MustCompile(`^(?i)-usuario=(?:"([^"]+)"|([^\s"]+))$`)
	rFlagRegex := regexp.MustCompile(`^(?i)-r$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -path y -usuario")
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

		if match = pathRegex.FindStringSubmatch(token); match != nil {
			key = "-path"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if match = usuarioRegex.FindStringSubmatch(token); match != nil {
			key = "-usuario"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if rFlagRegex.MatchString(token) {
			key = "-r"
			value = "true"
			matched = true
		}

		if !matched {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'", token)
		}
		fmt.Printf("  Match!: key='%s', value='%s'\n", key, value)

		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado: %s", key)
		}
		processedKeys[key] = true
		if value == "" && key != "-r" {
			return "", fmt.Errorf("el valor para %s no puede estar vacío", key)
		}

		switch key {
		case "-path":
			if !strings.HasPrefix(value, "/") {
				return "", fmt.Errorf("el path '%s' debe ser absoluto", value)
			}
			cmd.path = value
		case "-usuario":
			cmd.usuario = value
		case "-r":
			cmd.recursive = true
		}
	}

	// Verificar obligatorios
	if !processedKeys["-path"] {
		return "", errors.New("falta el parámetro requerido: -path")
	}
	if !processedKeys["-usuario"] {
		return "", errors.New("falta el parámetro requerido: -usuario")
	}

	// Llamar a la lógica del comando
	err := commandChown(cmd)
	if err != nil {
		return "", err
	}

	recursiveMsg := ""
	if cmd.recursive {
		recursiveMsg = " recursivamente"
	}
	return fmt.Sprintf("CHOWN: Propietario de '%s' cambiado a '%s'%s correctamente.", cmd.path, cmd.usuario, recursiveMsg), nil
}

func commandChown(cmd *CHOWN) error {
	fmt.Printf("Intentando cambiar propietario de '%s' a '%s' (Recursivo: %v)\n", cmd.path, cmd.usuario, cmd.recursive)

	// Autenticación y obtener SB/Partición
	if !stores.Auth.IsAuthenticated() { return errors.New("comando chown requiere inicio de sesión") }
	currentUser, _, partitionID := stores.Auth.GetCurrentUser()
	partitionSuperblock, _, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil { return fmt.Errorf("error obteniendo partición montada '%s': %w", partitionID, err) }
	if partitionSuperblock.S_magic != 0xEF53 { return errors.New("magia de superbloque inválida") }
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 { return errors.New("tamaño de inodo o bloque inválido") }

	// Obtener UID del Usuario Actual 
	var currentUserUID int32 = -1
	if currentUser != "root" {
		uid, _, errUID := getUserInfo(currentUser, partitionSuperblock, partitionPath) // Ignorar GID devuelto
		if errUID != nil { return fmt.Errorf("error crítico: no se pudo encontrar info del usuario logueado '%s': %w", currentUser, errUID) }
		currentUserUID = uid
		fmt.Printf("UID del usuario actual '%s': %d\n", currentUser, currentUserUID)
	} else {
		fmt.Println("Usuario actual es root.")
	}

	// Obtener UID del NUEVO Propietario
	fmt.Printf("Buscando UID para el nuevo propietario '%s'...\n", cmd.usuario)
	newOwnerUID, _, errNewUID := getUserInfo(cmd.usuario, partitionSuperblock, partitionPath) // Ignorar GID devuelto
	if errNewUID != nil { return fmt.Errorf("error: el nuevo propietario especificado '%s' no existe: %w", cmd.usuario, errNewUID) }
	fmt.Printf("UID del nuevo propietario '%s': %d\n", cmd.usuario, newOwnerUID)


	// Encontrar Inodo Objetivo
	fmt.Printf("Validando path objetivo: %s\n", cmd.path)
	targetInodeIndex, targetInode, errFind := structures.FindInodeByPath(partitionSuperblock, partitionPath, cmd.path)
	if errFind != nil { return fmt.Errorf("error: no se encontró '%s': %w", cmd.path, errFind) }
	fmt.Printf("Inodo objetivo encontrado: %d\n", targetInodeIndex)

	// Verificar Permiso para CAMBIAR Propietario
	fmt.Println("Verificando permiso para cambiar propietario...")
	canChangeOwner := false
	if currentUser == "root" { canChangeOwner = true; fmt.Println("  Permiso concedido (root).")
	} else {
		if targetInode.I_uid == currentUserUID { canChangeOwner = true; fmt.Printf("  Permiso concedido (usuario '%s' dueño inodo %d).\n", currentUser, targetInodeIndex) }
	}
	if !canChangeOwner { return fmt.Errorf("permiso denegado: solo dueño (UID %d) o root pueden cambiar propietario de '%s'", targetInode.I_uid, cmd.path) }

	// Llamar a la Función Recursiva
	fmt.Printf("Iniciando cambio de propietario recursivo (si aplica) desde inodo %d...\n", targetInodeIndex)
	errChown := recursiveChown(targetInodeIndex, newOwnerUID, partitionSuperblock, partitionPath, currentUser, currentUserUID, cmd.recursive)
	if errChown != nil { return fmt.Errorf("error durante cambio de propietario: %w", errChown) }



	if partitionSuperblock.S_filesystem_type == 3 { // Solo si la operación principal y recursión
		journalEntryData := structures.Information{
			I_operation: utils.StringToBytes10("chown"),
			I_path:      utils.StringToBytes32(cmd.path),         // Path afectado
			I_content:   utils.StringToBytes64(cmd.usuario),      // Nuevo usuario como contenido
		}
		errJournal := utils.AppendToJournal(journalEntryData, partitionSuperblock, partitionPath)
		if errJournal != nil {
			fmt.Printf("Advertencia: Falla al escribir en journal para chown '%s': %v\n", cmd.path, errJournal)
		}
	}





	fmt.Println("CHOWN completado exitosamente.")
	return nil
}

func recursiveChown(
	inodeIndex int32,
	newOwnerUID int32, // UID numérico del NUEVO dueño
	sb *structures.SuperBlock,
	diskPath string,
	currentUser string, // Nombre del usuario ejecutando
	currentUserUID int32, // UID del usuario ejecutando 
	applyRecursively bool, // Flag -r original
) error {
	fmt.Printf("--> recursiveChown: Procesando inodo %d, nuevo dueño UID %d\n", inodeIndex, newOwnerUID)

	// Validar Índice y Leer Inodo
	if inodeIndex < 0 || inodeIndex >= sb.S_inodes_count {
		return fmt.Errorf("índice inválido %d", inodeIndex)
	}
	inode := &structures.Inode{}
	inodeOffset := int64(sb.S_inode_start + inodeIndex*sb.S_inode_size)
	if err := inode.Deserialize(diskPath, inodeOffset); err != nil {
		return fmt.Errorf("no se pudo leer inodo %d: %w", inodeIndex, err)
	}

	// Verificar Permiso para CAMBIAR este inodo específico (solo si no somos root)
	canChangeThis := false
	if currentUser == "root" {
		canChangeThis = true
	} else {
		// Usuario normal solo puede cambiar si es el dueño ACTUAL
		if inode.I_uid == currentUserUID {
			canChangeThis = true
		}
	}

	if !canChangeThis {
		// No tenemos permiso sobre este item, lo omitimos (y a sus hijos si es dir)
		fmt.Printf("    Permiso denegado para chown en inodo %d (Dueño actual: %d, Usuario: %s/%d). Omitiendo.\n", inodeIndex, inode.I_uid, currentUser, currentUserUID)
		return nil
	}
	fmt.Printf("    Permiso concedido para chown en inodo %d.\n", inodeIndex)

	// Cambiar Dueño
	ownerChanged := false
	if inode.I_uid != newOwnerUID {
		fmt.Printf("    Cambiando I_uid de %d a %d\n", inode.I_uid, newOwnerUID)
		inode.I_uid = newOwnerUID
		inode.I_ctime = float32(time.Now().Unix()) // Actualizar ctime 
		ownerChanged = true
	} else {
		fmt.Println("    El inodo ya pertenece al nuevo dueño.")
	}
	// Actualizar atime porque lo leímos/modificamos
	inode.I_atime = float32(time.Now().Unix())

	// Serializar Inodo Modificado (si cambió o por atime)
	if err := inode.Serialize(diskPath, inodeOffset); err != nil {
		return fmt.Errorf("error crítico guardando inodo %d actualizado: %w", inodeIndex, err)
	}
	if ownerChanged {
		fmt.Println("    Inodo actualizado con nuevo dueño.")
	}

	// Recurrir si es Directorio y aplica
	if inode.I_type[0] == '0' && applyRecursively {
		fmt.Printf("    Inodo %d es DIRECTORIO, procesando contenido recursivamente...\n", inodeIndex)
		for i := 0; i < 12; i++ { // Solo directos
			blockPtr := inode.I_block[i]
			if blockPtr == -1 || blockPtr < 0 || blockPtr >= sb.S_blocks_count {
				continue
			}

			folderBlock := structures.FolderBlock{}
			blockOffset := int64(sb.S_block_start + blockPtr*sb.S_block_size)
			if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
				fmt.Printf("      Advertencia: Error leyendo bloque %d dir %d: %v\n", blockPtr, inodeIndex, err)
				continue
			}

			for j := range folderBlock.B_content {
				entry := folderBlock.B_content[j]
				if entry.B_inodo == -1 {
					continue
				}
				entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
				if entryName == "." || entryName == ".." {
					continue
				}

				fmt.Printf("        Llamando recursiveChown para hijo '%s' (inodo %d)...\n", entryName, entry.B_inodo)
				// Llamada RECURSIVA
				errRec := recursiveChown(entry.B_inodo, newOwnerUID, sb, diskPath, currentUser, currentUserUID, true) // Siempre recursivo para hijos
				if errRec != nil {
					fmt.Printf("        ERROR retornando de recursión en '%s': %v\n", entryName, errRec)
					return errRec
				}
				fmt.Printf("        Procesamiento recursivo de '%s' OK.\n", entryName)
			}
		}
	}

	fmt.Printf("<-- recursiveChown: Inodo %d procesado.\n", inodeIndex)
	return nil
}

