package commands

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time" // Para actualizar ctime

	stores "backend/stores"
	structures "backend/structures"
)

type CHMOD struct {
	path      string // Path absoluto al archivo/carpeta
	ugo       string // Permisos en formato string "UGO" (e.g., "764")
	recursive bool   // Flag -r
}

func ParseChmod(tokens []string) (string, error) {
	cmd := &CHMOD{} // recursive es false por defecto
	processedKeys := make(map[string]bool)

	pathRegex := regexp.MustCompile(`^(?i)-path=(?:"([^"]+)"|([^\s"]+))$`)
	ugoRegex := regexp.MustCompile(`^(?i)-ugo=([0-7]{3})$`) // Exactamente 3 dígitos 0-7
	rFlagRegex := regexp.MustCompile(`^(?i)-r$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -path y -ugo")
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
		} else if match = ugoRegex.FindStringSubmatch(token); match != nil {
			key = "-ugo"
			value = match[1]
			matched = true // Grupo 1 captura los 3 dígitos
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
		// No validar valor vacío para -r, para otros sí
		if value == "" && key != "-r" {
			return "", fmt.Errorf("el valor para %s no puede estar vacío", key)
		}

		switch key {
		case "-path":
			if !strings.HasPrefix(value, "/") {
				return "", fmt.Errorf("el path '%s' debe ser absoluto", value)
			}
			cmd.path = value
		case "-ugo":
			cmd.ugo = value
		case "-r":
			cmd.recursive = true
		}
	}

	if !processedKeys["-path"] {
		return "", errors.New("falta el parámetro requerido: -path")
	}
	if !processedKeys["-ugo"] {
		return "", errors.New("falta el parámetro requerido: -ugo")
	}

	err := commandChmod(cmd)
	if err != nil {
		return "", err
	}

	recursiveMsg := ""
	if cmd.recursive {
		recursiveMsg = " recursivamente"
	}
	return fmt.Sprintf("CHMOD: Permisos de '%s' cambiados a '%s'%s correctamente.", cmd.path, cmd.ugo, recursiveMsg), nil
}

func commandChmod(cmd *CHMOD) error {
	fmt.Printf("Intentando cambiar permisos de '%s' a '%s' (Recursivo: %v)\n", cmd.path, cmd.ugo, cmd.recursive)

	// Autenticación y obtener SB/Partición
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando chmod requiere inicio de sesión")
	}
	currentUser, _, partitionID := stores.Auth.GetCurrentUser() // Ignoramos GID string por ahora
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

	// Obtener UID del Usuario Actual (solo si no es root)
	var currentUserUID int32 = -1
	if currentUser != "root" {
		uid, _, errUID := getUserInfo(currentUser, partitionSuperblock, partitionPath) // Ignorar GID
		if errUID != nil {
			return fmt.Errorf("error crítico: no se pudo encontrar info del usuario logueado '%s': %w", currentUser, errUID)
		}
		currentUserUID = uid
		fmt.Printf("UID del usuario actual '%s': %d\n", currentUser, currentUserUID)
	} else {
		fmt.Println("Usuario actual es root.")
	}

	// Encontrar Inodo Objetivo
	fmt.Printf("Validando path objetivo: %s\n", cmd.path)
	targetInodeIndex, targetInode, errFind := structures.FindInodeByPath(partitionSuperblock, partitionPath, cmd.path)
	if errFind != nil {
		return fmt.Errorf("error: no se encontró el archivo o directorio '%s': %w", cmd.path, errFind)
	}
	fmt.Printf("Inodo objetivo encontrado: %d\n", targetInodeIndex)

	// Verificar Permiso para CAMBIAR Permisos (Dueño o Root)
	fmt.Println("Verificando permiso para cambiar permisos...")
	canChangePerms := false
	if currentUser == "root" {
		canChangePerms = true
		fmt.Println("  Permiso concedido (root).")
	} else {
		if targetInode.I_uid == currentUserUID { // Solo el dueño puede cambiar permisos
			canChangePerms = true
			fmt.Printf("  Permiso concedido (usuario '%s' es dueño del inodo %d).\n", currentUser, targetInodeIndex)
		}
	}
	if !canChangePerms {
		return fmt.Errorf("permiso denegado: solo el dueño (UID %d) o root pueden cambiar los permisos de '%s'", targetInode.I_uid, cmd.path)
	}

	// Convertir ugo string ("764") a byte array ([]byte{'7','6','4'})
	if len(cmd.ugo) != 3 {
		return errors.New("error interno: -ugo no tiene 3 dígitos")
	}
	newPerms := [3]byte{cmd.ugo[0], cmd.ugo[1], cmd.ugo[2]}

	// Llamar a la Función Recursiva
	fmt.Printf("Iniciando cambio de permisos recursivo (si aplica) desde inodo %d...\n", targetInodeIndex)
	errChmod := recursiveChmod(targetInodeIndex, newPerms, partitionSuperblock, partitionPath, currentUser, currentUserUID, cmd.recursive)
	if errChmod != nil {
		return fmt.Errorf("error durante el cambio de permisos: %w", errChmod)
	}

	// Serializar Superbloque (No necesario para chmod)

	fmt.Println("CHMOD completado exitosamente.")
	return nil
}

// Cambia los permisos de un inodo y recursivamente si es dir.
func recursiveChmod(
	inodeIndex int32,
	newPerms [3]byte, // Nuevos permisos como array de bytes (ej: {'7','6','4'})
	sb *structures.SuperBlock,
	diskPath string,
	currentUser string, // Nombre del usuario ejecutando
	currentUserUID int32, // UID del usuario ejecutando (-1 si es root)
	applyRecursively bool, // Flag -r original
) error {
	fmt.Printf("--> recursiveChmod: Procesando inodo %d, nuevos permisos %s\n", inodeIndex, string(newPerms[:]))

	// Validar Índice y Leer Inodo
	if inodeIndex < 0 || inodeIndex >= sb.S_inodes_count {
		return fmt.Errorf("índice inválido %d", inodeIndex)
	}
	inode := &structures.Inode{}
	inodeOffset := int64(sb.S_inode_start + inodeIndex*sb.S_inode_size)
	if err := inode.Deserialize(diskPath, inodeOffset); err != nil {
		return fmt.Errorf("no se pudo leer inodo %d: %w", inodeIndex, err)
	}

	// Verificar Permiso para CAMBIAR este inodo (Dueño o Root)
	canChangeThis := false
	if currentUser == "root" {
		canChangeThis = true
	} else {
		if inode.I_uid == currentUserUID {
			canChangeThis = true
		}
	}

	if !canChangeThis {
		fmt.Printf("    Permiso denegado para chmod en inodo %d (Dueño: %d, User: %d). Omitiendo.\n", inodeIndex, inode.I_uid, currentUserUID)
		return nil // Omitir, no es error fatal
	}
	fmt.Printf("    Permiso concedido para chmod en inodo %d.\n", inodeIndex)

	// Cambiar Permisos y Timestamp
	permsChanged := false
	if inode.I_perm != newPerms {
		fmt.Printf("    Cambiando I_perm de %s a %s\n", string(inode.I_perm[:]), string(newPerms[:]))
		inode.I_perm = newPerms
		inode.I_ctime = float32(time.Now().Unix()) // Actualizar ctime (cambio de metadato)
		permsChanged = true
	} else {
		fmt.Println("    Los permisos ya son los solicitados.")
	}
	// Actualizar atime porque accedimos/modificamos
	inode.I_atime = float32(time.Now().Unix())

	// Serializar Inodo Modificado (si cambió o por atime)
	if err := inode.Serialize(diskPath, inodeOffset); err != nil {
		return fmt.Errorf("error crítico guardando inodo %d actualizado: %w", inodeIndex, err)
	}
	if permsChanged {
		fmt.Println("    Inodo actualizado con nuevos permisos.")
	}

	// Recurrir si es Directorio y aplica
	if inode.I_type[0] == '0' && applyRecursively {
		fmt.Printf("    Inodo %d es DIR, procesando recursivamente...\n", inodeIndex)
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

				fmt.Printf("        Llamando recursiveChmod para hijo '%s' (inodo %d)...\n", entryName, entry.B_inodo)
				errRec := recursiveChmod(entry.B_inodo, newPerms, sb, diskPath, currentUser, currentUserUID, true)
				if errRec != nil {
					fmt.Printf("        ERROR retornando de recursión en '%s': %v\n", entryName, errRec)
					return errRec
				}
				fmt.Printf("        Procesamiento '%s' OK.\n", entryName)
			}
		}
	}

	fmt.Printf("<-- recursiveChmod: Inodo %d procesado.\n", inodeIndex)
	return nil
}
