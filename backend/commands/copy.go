package commands

import (
	"errors"
	"fmt"
	"path/filepath" // Para Base/Dir
	"regexp"
	"strings"
	"time" // Para timestamps

	stores "backend/stores"
	structures "backend/structures"
)

type COPY struct {
	Path    string // Path de origen DENTRO del FS
	Destino string // Path del directorio destino DENTRO del FS
}

func ParseCopy(tokens []string) (string, error) {
	cmd := &COPY{}
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
		if match = pathRegex.FindStringSubmatch(token); match != nil {
			key = "-path"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if match = destinoRegex.FindStringSubmatch(token); match != nil {
			key = "-destino"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		}
		if !matched {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -path= o -destino=", token)
		}
		fmt.Printf("  Match!: key='%s', value='%s'\n", key, value)
		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado: %s", key)
		}
		processedKeys[key] = true
		if value == "" {
			return "", fmt.Errorf("el valor para %s no puede estar vacío", key)
		}
		switch key {
		case "-path":
			if !strings.HasPrefix(value, "/") {
				return "", fmt.Errorf("el path '%s' debe ser absoluto", value)
			}
			cmd.Path = value
		case "-destino":
			if !strings.HasPrefix(value, "/") {
				return "", fmt.Errorf("el path de destino '%s' debe ser absoluto", value)
			}
			cmd.Destino = value
		}
	}
	if !processedKeys["-path"] {
		return "", errors.New("falta -path")
	}
	if !processedKeys["-destino"] {
		return "", errors.New("falta -destino")
	}
	if cmd.Path == cmd.Destino {
		return "", errors.New("origen y destino no pueden ser iguales")
	}
	if strings.HasPrefix(cmd.Destino, cmd.Path+"/") || cmd.Destino == cmd.Path {
		if cmd.Path != "/" {
			return "", fmt.Errorf("destino '%s' no puede estar dentro del origen '%s'", cmd.Destino, cmd.Path)
		}
	}

	err := commandCopy(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("COPY: '%s' copiado a '%s' correctamente (con posibles omisiones por permisos).", cmd.Path, cmd.Destino), nil
}

func commandCopy(cmd *COPY) error {
	fmt.Printf("Intentando copiar '%s' a '%s'\n", cmd.Path, cmd.Destino)

	// Autenticación y obtener SB/Partición
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando copy requiere inicio de sesión")
	}
	currentUser, userGIDStr, partitionID := stores.Auth.GetCurrentUser()
	partitionSuperblock, mountedPartition, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID) // Obtener mountedPartition aquí
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
	fmt.Printf("Validando origen: %s\n", cmd.Path)
	sourceInodeIndex, sourceInode, errFindSource := structures.FindInodeByPath(partitionSuperblock, partitionPath, cmd.Path)
	if errFindSource != nil {
		return fmt.Errorf("error: no se encontró el origen '%s': %w", cmd.Path, errFindSource)
	}
	if !checkPermissions(currentUser, userGIDStr, 'r', sourceInode, partitionSuperblock, partitionPath) {
		return fmt.Errorf("permiso denegado: lectura sobre origen '%s'", cmd.Path)
	}
	fmt.Println("Permiso de lectura sobre origen concedido.")

	// Validar Destino 
	fmt.Printf("Validando destino: %s\n", cmd.Destino)
	destDirInodeIndex, destDirInode, errFindDest := structures.FindInodeByPath(partitionSuperblock, partitionPath, cmd.Destino)
	if errFindDest != nil {
		return fmt.Errorf("error: no se encontró directorio destino '%s': %w", cmd.Destino, errFindDest)
	}
	if destDirInode.I_type[0] != '0' {
		return fmt.Errorf("error: destino '%s' no es directorio", cmd.Destino)
	}
	if !checkPermissions(currentUser, userGIDStr, 'w', destDirInode, partitionSuperblock, partitionPath) {
		return fmt.Errorf("permiso denegado: escritura sobre directorio destino '%s'", cmd.Destino)
	}
	fmt.Println("Permiso de escritura sobre destino concedido.")

	// Verificar si ya existe algo con el mismo nombre en el destino
	sourceBaseName := filepath.Base(cmd.Path)
	fmt.Printf("Verificando si '%s' ya existe en destino '%s'...\n", sourceBaseName, cmd.Destino)
	exists, _, _ := findEntryInParent(destDirInode, sourceBaseName, partitionSuperblock, partitionPath)
	if exists {
		return fmt.Errorf("error: '%s' ya existe en destino '%s'", sourceBaseName, cmd.Destino)
	}
	fmt.Printf("Nombre '%s' disponible en destino.\n", sourceBaseName)

	// Llamar a la Función Recursiva de Copia
	fmt.Printf("Iniciando copia recursiva de inodo %d a padre destino %d como '%s'...\n", sourceInodeIndex, destDirInodeIndex, sourceBaseName)
	errCopy := recursiveCopy(sourceInodeIndex, destDirInodeIndex, sourceBaseName, partitionSuperblock, partitionPath, currentUser, userGIDStr)
	if errCopy != nil {
		return fmt.Errorf("error durante la copia: %w", errCopy)
	}

	// Actualizar Timestamps Destino
	fmt.Println("Actualizando timestamp del directorio destino...")
	destDirInode.I_mtime = float32(time.Now().Unix())
	destDirInode.I_atime = destDirInode.I_mtime
	destDirInodeOffset := int64(partitionSuperblock.S_inode_start + destDirInodeIndex*partitionSuperblock.S_inode_size)
	if err := destDirInode.Serialize(partitionPath, destDirInodeOffset); err != nil {
		fmt.Printf("Advertencia: Error guardando inodo destino %d actualizado: %v\n", destDirInodeIndex, err)
	}

	// Serializar Superbloque
	fmt.Println("Serializando SuperBlock después de COPY...")
	// Eliminar las líneas incorrectas que intentaban parsear el path
	// Usar la variable 'mountedPartition' obtenida al principio
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		return fmt.Errorf("ADVERTENCIA: error al serializar superbloque después de copy: %w", err)
	}

	fmt.Println("COPY completado.")
	return nil
}

// recursiveCopy 
func recursiveCopy(
	sourceInodeIndex int32,
	parentDestInodeIndex int32,
	newName string,
	sb *structures.SuperBlock,
	diskPath string,
	currentUser string,
	userGIDStr string,
) error {
	fmt.Printf("--> recursiveCopy: Copiando inodo origen %d a padre destino %d como '%s'\n", sourceInodeIndex, parentDestInodeIndex, newName)

	// Leer inodo origen
	sourceInode := &structures.Inode{}
	sourceInodeOffset := int64(sb.S_inode_start + sourceInodeIndex*sb.S_inode_size)
	if err := sourceInode.Deserialize(diskPath, sourceInodeOffset); err != nil {
		return fmt.Errorf("no se pudo leer inodo origen %d: %w", sourceInodeIndex, err)
	}

	// Verificar Permiso de LECTURA en origen
	if !checkPermissions(currentUser, userGIDStr, 'r', sourceInode, sb, diskPath) {
		fmt.Printf("    Permiso de lectura denegado en origen (inodo %d). Omitiendo copia de '%s'.\n", sourceInodeIndex, newName)
		return nil // Omitir
	}
	fmt.Printf("    Permiso de lectura concedido para inodo origen %d.\n", sourceInodeIndex)

	// Procesar según tipo
	if sourceInode.I_type[0] == '1' { // ARCHIVO
		fmt.Printf("    Origen es ARCHIVO. Procediendo a copiar...\n")
		// Leer contenido
		contentBytesStr, errRead := structures.ReadFileContent(sb, diskPath, sourceInode)
		if errRead != nil {
			return fmt.Errorf("error leyendo contenido origen %d: %w", sourceInodeIndex, errRead)
		}
		contentBytes := []byte(contentBytesStr)
		newSize := int32(len(contentBytes))
		fmt.Printf("      Contenido leído: %d bytes.\n", newSize)
		// Calcular bloques y verificar espacio
		blockSize := sb.S_block_size
		numBlocksNeeded := int32(0)
		if newSize > 0 {
			numBlocksNeeded = (newSize + blockSize - 1) / blockSize
		}
		if numBlocksNeeded > sb.S_free_blocks_count {
			return fmt.Errorf("espacio insuficiente: necesita %d bloques, libres %d", numBlocksNeeded, sb.S_free_blocks_count)
		}
		// Asignar bloques y escribir contenido
		fmt.Printf("      Asignando %d bloques y escribiendo...\n", numBlocksNeeded)
		newAllocatedBlockIndices, errAlloc := allocateDataBlocks(contentBytes, newSize, sb, diskPath)
		if errAlloc != nil {
			return fmt.Errorf("falló asignación/escritura copia archivo: %w", errAlloc)
		}
		// Asignar nuevo inodo
		newInodeIndex, errInodeAlloc := sb.FindFreeInode(diskPath)
		if errInodeAlloc != nil {
			return fmt.Errorf("no se pudo asignar inodo copia archivo: %w", errInodeAlloc)
		}
		if err := sb.UpdateBitmapInode(diskPath, newInodeIndex, '1'); err != nil {
			return fmt.Errorf("error bitmap inodo copia %d: %w", newInodeIndex, err)
		}
		sb.S_free_inodes_count--
		fmt.Printf("      Nuevo inodo asignado: %d\n", newInodeIndex)
		// Crear y serializar nuevo inodo
		currentTime := float32(time.Now().Unix())
		newInode := &structures.Inode{I_uid: sourceInode.I_uid, I_gid: sourceInode.I_gid, I_size: newSize, I_atime: currentTime, I_ctime: currentTime, I_mtime: currentTime /* O mantener sourceInode.I_mtime */, I_type: [1]byte{'1'}, I_perm: sourceInode.I_perm, I_block: newAllocatedBlockIndices}
		newInodeOffset := int64(sb.S_inode_start + newInodeIndex*sb.S_inode_size)
		if err := newInode.Serialize(diskPath, newInodeOffset); err != nil {
			return fmt.Errorf("error serializando nuevo inodo copia %d: %w", newInodeIndex, err)
		}
		// Añadir entrada al directorio destino
		fmt.Printf("      Añadiendo entrada '%s' a dir destino %d...\n", newName, parentDestInodeIndex)
		errAdd := addEntryToParent(parentDestInodeIndex, newName, newInodeIndex, sb, diskPath)
		if errAdd != nil {
			return fmt.Errorf("error añadiendo entrada archivo copiado '%s': %w", newName, errAdd)
		}

	} else if sourceInode.I_type[0] == '0' { //  DIRECTORIO 
		fmt.Printf("    Origen es DIRECTORIO. Copiando recursivamente...\n")
		// Asignar nuevo inodo
		newDirInodeIndex, errInodeAlloc := sb.FindFreeInode(diskPath)
		if errInodeAlloc != nil {
			return fmt.Errorf("no se pudo asignar inodo copia dir '%s': %w", newName, errInodeAlloc)
		}
		if err := sb.UpdateBitmapInode(diskPath, newDirInodeIndex, '1'); err != nil {
			return fmt.Errorf("error bitmap inodo dir copia %d: %w", newDirInodeIndex, err)
		}
		sb.S_free_inodes_count--
		fmt.Printf("      Nuevo inodo asignado: %d\n", newDirInodeIndex)
		// Asignar nuevo bloque
		newDirBlockIndex, errBlockAlloc := sb.FindFreeBlock(diskPath)
		if errBlockAlloc != nil {
			return fmt.Errorf("no se pudo asignar bloque copia dir '%s': %w", newName, errBlockAlloc)
		}
		if err := sb.UpdateBitmapBlock(diskPath, newDirBlockIndex, '1'); err != nil {
			return fmt.Errorf("error bitmap bloque dir copia %d: %w", newDirBlockIndex, err)
		}
		sb.S_free_blocks_count--
		fmt.Printf("      Nuevo bloque asignado: %d\n", newDirBlockIndex)
		// Crear y serializar nuevo inodo dir
		currentTime := float32(time.Now().Unix())
		newDirInode := &structures.Inode{I_uid: sourceInode.I_uid, I_gid: sourceInode.I_gid, I_size: 0, I_atime: currentTime, I_ctime: currentTime, I_mtime: currentTime, I_type: [1]byte{'0'}, I_perm: sourceInode.I_perm}
		for i := range newDirInode.I_block {
			newDirInode.I_block[i] = -1
		}
		newDirInode.I_block[0] = newDirBlockIndex
		newDirInodeOffset := int64(sb.S_inode_start + newDirInodeIndex*sb.S_inode_size)
		if err := newDirInode.Serialize(diskPath, newDirInodeOffset); err != nil {
			return fmt.Errorf("error serializando nuevo inodo dir copia %d: %w", newDirInodeIndex, err)
		}
		// Crear y serializar nuevo bloque dir
		newDirFolderBlock := structures.FolderBlock{}
		newDirFolderBlock.Initialize()
		copy(newDirFolderBlock.B_content[0].B_name[:], ".")
		newDirFolderBlock.B_content[0].B_inodo = newDirInodeIndex
		copy(newDirFolderBlock.B_content[1].B_name[:], "..")
		newDirFolderBlock.B_content[1].B_inodo = parentDestInodeIndex
		newDirBlockOffset := int64(sb.S_block_start + newDirBlockIndex*sb.S_block_size)
		if err := newDirFolderBlock.Serialize(diskPath, newDirBlockOffset); err != nil {
			return fmt.Errorf("error serializando nuevo bloque dir copia %d: %w", newDirBlockIndex, err)
		}
		// Añadir entrada para este dir en su padre destino
		fmt.Printf("      Añadiendo entrada '%s' a dir destino padre %d...\n", newName, parentDestInodeIndex)
		errAdd := addEntryToParent(parentDestInodeIndex, newName, newDirInodeIndex, sb, diskPath)
		if errAdd != nil {
			return fmt.Errorf("error añadiendo entrada dir copiado '%s': %w", newName, errAdd)
		}
		// Iterar sobre contenido del dir ORIGEN
		fmt.Printf("      Iterando contenido dir origen %d...\n", sourceInodeIndex)
		for i := 0; i < 12; i++ {
			blockPtr := sourceInode.I_block[i]
			if blockPtr == -1 || blockPtr < 0 || blockPtr >= sb.S_blocks_count {
				continue
			}
			folderBlock := structures.FolderBlock{}
			blockOffset := int64(sb.S_block_start + blockPtr*sb.S_block_size)
			if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
				fmt.Printf("      Adv: Error leyendo bloque %d origen: %v.\n", blockPtr, err)
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
				fmt.Printf("        Llamando recursiveCopy para hijo '%s' (inodo %d) DENTRO de nuevo dir %d...\n", entryName, entry.B_inodo, newDirInodeIndex)
				errRec := recursiveCopy(entry.B_inodo, newDirInodeIndex, entryName, sb, diskPath, currentUser, userGIDStr)
				if errRec != nil {
					fmt.Printf("        ERROR (omitido según spec): Falla al copiar '%s': %v\n", entryName, errRec) /* NO return errRec */
				} else {
					fmt.Printf("        Copia '%s' OK.\n", entryName)
				}
			}
		}
	} else {
		fmt.Printf("    Advertencia: Inodo origen %d tipo desconocido '%c'. Omitiendo.\n", sourceInodeIndex, sourceInode.I_type[0])
	}
	fmt.Printf("<-- recursiveCopy: Procesamiento inodo origen %d completado.\n", sourceInodeIndex)
	return nil
}

// Verifica permisos 
func checkPermissions(currentUser string, userGroupStr string, requiredPermission byte, targetInode *structures.Inode, _ *structures.SuperBlock, _ string) bool {
	fmt.Printf("      checkPermissions: User='%s' GroupStr='%s' Req='%c' on Inode (UID=%d, GID=%d, Perm=%s)\n",
		currentUser, userGroupStr, requiredPermission, targetInode.I_uid, targetInode.I_gid, string(targetInode.I_perm[:]))

	// Caso Root
	if currentUser == "root" {
		fmt.Println("        Permiso concedido (root).")
		return true
	}

	//Obtener UID y GID numéricos del usuario actual (DESDE users.txt)
	currentUserUID := int32(-1)
	currentUserGID := int32(-1)
	if currentUser != "root" {
		currentUserUID = 1 
		currentUserGID = 1 
		fmt.Printf("        Advertencia: Usando UID/GID placeholder (%d/%d) para '%s'\n", currentUserUID, currentUserGID, currentUser)
	}

	// Extraer permisos del inodo
	ownerPerms := targetInode.I_perm[0]
	groupPerms := targetInode.I_perm[1]
	otherPerms := targetInode.I_perm[2]

	// Verificar permisos
	hasPerm := false

	// Chequear Dueño
	if targetInode.I_uid == currentUserUID {
		fmt.Println("        Usuario es dueño.")
		switch requiredPermission {
		case 'r':
			hasPerm = (ownerPerms == 'r' || ownerPerms == '4' || ownerPerms == '5' || ownerPerms == '6' || ownerPerms == '7')
		case 'w':
			hasPerm = (ownerPerms == 'w' || ownerPerms == '2' || ownerPerms == '3' || ownerPerms == '6' || ownerPerms == '7')
		case 'x':
			hasPerm = (ownerPerms == 'x' || ownerPerms == '1' || ownerPerms == '3' || ownerPerms == '5' || ownerPerms == '7')
		}
		if hasPerm {
			fmt.Println("        Permiso de dueño concedido.")
			return true
		}
	}

	// Chequear Grupo
	if targetInode.I_gid == currentUserGID { 
		fmt.Println("        Usuario pertenece al grupo.")
		switch requiredPermission {
		case 'r':
			hasPerm = (groupPerms == 'r' || groupPerms == '4' || groupPerms == '5' || groupPerms == '6' || groupPerms == '7')
		case 'w':
			hasPerm = (groupPerms == 'w' || groupPerms == '2' || groupPerms == '3' || groupPerms == '6' || groupPerms == '7')
		case 'x':
			hasPerm = (groupPerms == 'x' || groupPerms == '1' || groupPerms == '3' || groupPerms == '5' || groupPerms == '7')
		}
		if hasPerm {
			fmt.Println("        Permiso de grupo concedido.")
			return true
		}
	}

	fmt.Println("        Verificando permisos de 'otros'.")
	switch requiredPermission {
	case 'r':
		hasPerm = (otherPerms == 'r' || otherPerms == '4' || otherPerms == '5' || otherPerms == '6' || otherPerms == '7')
	case 'w':
		hasPerm = (otherPerms == 'w' || otherPerms == '2' || otherPerms == '3' || otherPerms == '6' || otherPerms == '7')
	case 'x':
		hasPerm = (otherPerms == 'x' || otherPerms == '1' || otherPerms == '3' || otherPerms == '5' || otherPerms == '7')
	}
	if hasPerm {
		fmt.Println("        Permiso de 'otros' concedido.")
		return true
	}

	// Si no se concedió en ninguna etapa
	fmt.Println("        Permiso denegado.")
	return false
}