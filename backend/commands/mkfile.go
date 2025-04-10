package commands

import (
	"fmt"
	"os" // Necesario para leer archivo con -cont
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	stores "backend/stores"
	structures "backend/structures"
	utils "backend/utils"
	"errors"
)

type MKFILE struct {
	path string // Path del archivo
	r    bool   // Crear padres recursivamente
	size int    // Tamaño en bytes (si no se usa -cont)
	cont string // Path al archivo local con contenido
}

// ParseMkfile analiza los tokens para el comando mkfile
func ParseMkfile(tokens []string) (string, error) {
	cmd := &MKFILE{size: 0} // Inicializar tamaño a 0 por defecto

	args := strings.Join(tokens, " ")
	// Expresión regular mejorada para capturar valores con/sin comillas y flags
	re := regexp.MustCompile(`-(path|cont)=("[^"]+"|[^\s]+)|-size=(\d+)|(-r)`)
	matches := re.FindAllStringSubmatch(args, -1) // Usar Submatch para capturar grupos

	parsedArgs := make(map[string]bool) // Para rastrear qué parte del string original ya se procesó
	for _, match := range matches {
		fullMatch := match[0]
		parsedArgs[fullMatch] = true     // Marcar el token completo como procesado
		key := strings.ToLower(match[1]) // path o cont (grupo 1)
		value := match[2]                // Valor para path/cont (grupo 2)
		sizeStr := match[3]              // Valor para size (grupo 3)
		flagR := match[4]                // -r (grupo 4)

		// Limpiar comillas del valor si existen
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		switch {
		case key == "path":
			cmd.path = value
		case key == "cont":
			cmd.cont = value
		case sizeStr != "":
			size, err := strconv.Atoi(sizeStr)
			if err != nil {
				return "", fmt.Errorf("valor de -size inválido: %s", sizeStr)
			}
			if size < 0 {
				return "", errors.New("el valor de -size no puede ser negativo")
			}
			cmd.size = size
		case flagR == "-r":
			cmd.r = true
		}
	}

	// Verificar si hubo tokens no reconocidos
	originalTokens := strings.Fields(args)
	for _, token := range originalTokens {
		isProcessed := false
		for parsed := range parsedArgs {
			if strings.Contains(parsed, token) || strings.Contains(token, parsed) {
				isProcessed = true
				break
			}
		}
		if token == "-r" {
			isProcessed = true
		}

		if !isProcessed && !strings.HasPrefix(token, "-") {
		} else if !isProcessed {
			return "", fmt.Errorf("parámetro o formato inválido cerca de: %s", token)
		}
	}
	// Validaciones obligatorias
	if cmd.path == "" {
		return "", errors.New("parámetro obligatorio faltante: -path")
	}
	if cmd.cont != "" && cmd.size != 0 && len(matches) > 0 {
		fmt.Println("Parámetro -size ignorado porque -cont fue proporcionado.")
		cmd.size = 0
	}
	// Validar existencia de archivo en -cont si se proporcionó
	if cmd.cont != "" {
		if _, err := os.Stat(cmd.cont); os.IsNotExist(err) {
			return "", fmt.Errorf("el archivo especificado en -cont no existe: %s", cmd.cont)
		}
	}
	err := commandMkfile(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("MKFILE: Archivo '%s' creado correctamente.", cmd.path), nil
}

func commandMkfile(mkfile *MKFILE) error {
	// Obtener Autenticación y Partición Montada
	var userID int32 = 1
	var groupID int32 = 1
	var partitionID string

	if stores.Auth.IsAuthenticated() {
		partitionID = stores.Auth.GetPartitionID()
		fmt.Printf("Usuario autenticado: %s (Usando UID=%d, GID=%d)\n", stores.Auth.Username, userID, groupID)
	} else {
		return errors.New("comando mkfile requiere sesión iniciada (login)")
	}

	partitionSuperblock, mountedPartition, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return fmt.Errorf("error al obtener la partición montada '%s': %w", partitionID, err)
	}

	// Validar tamaños para división por cero o valores inválidos
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return fmt.Errorf("tamaño de inodo o bloque inválido en superbloque: inode=%d, block=%d", partitionSuperblock.S_inode_size, partitionSuperblock.S_block_size)
	}
	if partitionSuperblock.S_magic != 0xEF53 {
		return fmt.Errorf("magia del superbloque inválida (0x%X), posible corrupción o formato incorrecto", partitionSuperblock.S_magic)
	}

	// Limpiar Path y Obtener Padre/Nombre
	cleanPath := strings.TrimSuffix(mkfile.path, "/")
	if !strings.HasPrefix(cleanPath, "/") {
		return errors.New("el path debe ser absoluto (empezar con /)")
	}
	//Limpiar el path de caracteres inválidos
	if cleanPath == "/" {
		return errors.New("no se puede crear archivo en la raíz '/' con este comando, use un subdirectorio")
	}
	// Verificar que no haya caracteres inválidos
	if cleanPath == "" {
		return errors.New("el path no puede estar vacío")
	}

	// Verificar que no haya caracteres inválidos
	parentPath := filepath.Dir(cleanPath)
	if parentPath == "." || parentPath == "" {
		parentPath = "/"
	}

	// Verificar que el padre no sea la raíz
	fileName := filepath.Base(cleanPath)
	if fileName == "" || fileName == "." || fileName == ".." {
		return fmt.Errorf("nombre de archivo inválido: '%s'", fileName)
	}
	// Verificar que el nombre no contenga la cantidad de caracteres inválidos
	if len(fileName) > 11 {
		return fmt.Errorf("el nombre del archivo '%s' excede los 11 caracteres permitidos (máx 12 bytes con nulo)", fileName)
	}

	fmt.Printf("Asegurando directorio padre: %s\n", parentPath)
	parentInodeIndex, parentInode, err := ensureParentDirExists(parentPath, mkfile.r, partitionSuperblock, partitionPath)
	if err != nil {
		return err
	}

	// Verificar si el nombre ya existe en el padre
	fmt.Printf("Verificando si '%s' ya existe en directorio padre (inodo %d)...\n", fileName, parentInodeIndex)
	exists, _, existingInodeType := findEntryInParent(parentInode, fileName, partitionSuperblock, partitionPath)
	if exists {
		existingTypeStr := "elemento"
		if existingInodeType == '0' {
			existingTypeStr = "directorio"
		} else if existingInodeType == '1' {
			existingTypeStr = "archivo"
		}
		return fmt.Errorf("error: el %s '%s' ya existe en '%s'", existingTypeStr, fileName, parentPath)
	}

	// Determinar Contenido y Tamaño Final
	var contentBytes []byte
	var fileSize int32

	// Leer contenido desde archivo local o generar contenido
	if mkfile.cont != "" {
		fmt.Printf("Leyendo contenido desde archivo local: %s\n", mkfile.cont)
		hostContent, errRead := os.ReadFile(mkfile.cont)
		if errRead != nil {
			return fmt.Errorf("error leyendo archivo de contenido '%s': %w", mkfile.cont, errRead)
		}
		contentBytes = hostContent
		fileSize = int32(len(contentBytes))
	} else {
		// Generar contenido basado en el tamaño
		fileSize = int32(mkfile.size)

		if fileSize > 0 {
			fmt.Printf("Generando contenido de %d bytes (0-9 repetido)...\n", fileSize)
			contentBuilder := strings.Builder{}
			contentBuilder.Grow(int(fileSize))
			for i := int32(0); i < fileSize; i++ {
				contentBuilder.WriteByte(byte('0' + (i % 10)))
			}
			contentBytes = []byte(contentBuilder.String())
		} else {
			// Tamaño 0, archivo vacío
			contentBytes = []byte{}
		}
	}
	fmt.Printf("Tamaño final del archivo: %d bytes\n", fileSize)

	// Calcular bloques necesarios
	blockSize := partitionSuperblock.S_block_size
	numBlocksNeeded := int32(0)
	if fileSize > 0 {
		// Manejo división por cero si blockSize fuera inválido (aunque ya se validó SB)
		if blockSize <= 0 {
			return errors.New("tamaño de bloque inválido en superbloque")
		}
		numBlocksNeeded = (fileSize + blockSize - 1) / blockSize
	} else {
		numBlocksNeeded = 0
	}

	// Verificar si hay suficientes bloques libres
	fmt.Printf("Asignando %d bloque(s) de datos y punteros necesarios...\n", numBlocksNeeded)
	var allocatedBlockIndices [15]int32
	allocatedBlockIndices, err = allocateDataBlocks(contentBytes, fileSize, partitionSuperblock, partitionPath)
	if err != nil {
		return fmt.Errorf("falló la asignación de bloques: %w", err)
	}

	// Asignar Inodo
	fmt.Println("Asignando inodo...")
	newInodeIndex, err := partitionSuperblock.FindFreeInode(partitionPath)
	if err != nil {
		return fmt.Errorf("no se pudo asignar un nuevo inodo: %w", err)
	}

	// Validar índice de inodo
	fmt.Printf("Inodo libre encontrado: %d\n", newInodeIndex)
	err = partitionSuperblock.UpdateBitmapInode(partitionPath, newInodeIndex,'1')
	if err != nil {
		return fmt.Errorf("error actualizando bitmap para inodo %d: %w", newInodeIndex, err)
	}
	partitionSuperblock.S_free_inodes_count-- // Decrementar contador global

	// Crear y Serializar Estructura Inodo
	currentTime := float32(time.Now().Unix())
	newInode := &structures.Inode{
		I_uid:   userID,
		I_gid:   groupID,
		I_size:  fileSize,
		I_atime: currentTime,
		I_ctime: currentTime,
		I_mtime: currentTime,
		I_type:  [1]byte{'1'},           // '1' para archivo
		I_perm:  [3]byte{'6', '6', '4'}, // Permisos rw-rw-r--
	}
	// Copiar los índices de bloques asignados
	newInode.I_block = allocatedBlockIndices

	// Calcular offset y serializar
	inodeOffset := int64(partitionSuperblock.S_inode_start) + int64(newInodeIndex)*int64(partitionSuperblock.S_inode_size)
	fmt.Printf("Serializando nuevo inodo %d en offset %d...\n", newInodeIndex, inodeOffset)
	err = newInode.Serialize(partitionPath, inodeOffset)
	if err != nil {
		return fmt.Errorf("error serializando nuevo inodo %d: %w", newInodeIndex, err)
	}

	// Añadir Entrada al Directorio Padre
	fmt.Printf("Añadiendo entrada '%s' al directorio padre (inodo %d)...\n", fileName, parentInodeIndex)
	err = addEntryToParent(parentInodeIndex, fileName, newInodeIndex, partitionSuperblock, partitionPath)
	if err != nil {
		return fmt.Errorf("error añadiendo entrada '%s' al directorio padre: %w", fileName, err)
	}

	// Serializar el contenido en los bloques asignados
	fmt.Println("\nSerializando SuperBlock después de MKFILE...")
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		return fmt.Errorf("ADVERTENCIA: error al serializar el superbloque después de mkfile, los cambios podrían perderse (%w)", err)
	}

	fmt.Println("MKFILE completado exitosamente.")
	return nil
}

// Retorna el índice y el inodo del padre directo si todo va bien.
func ensureParentDirExists(targetParentPath string, createRecursively bool, sb *structures.SuperBlock, partitionPath string) (int32, *structures.Inode, error) {
	fmt.Printf("Asegurando que exista: %s (Recursivo: %v)\n", targetParentPath, createRecursively)
	//El padre es la raíz "/"
	if targetParentPath == "/" {
		inode := &structures.Inode{}
		offset := int64(sb.S_inode_start) // Raíz es inodo 0
		err := inode.Deserialize(partitionPath, offset)
		if err != nil {
			return -1, nil, fmt.Errorf("error crítico: no se pudo deserializar inodo raíz (0): %w", err)
		}
		if inode.I_type[0] != '0' {
			return -1, nil, errors.New("error crítico: inodo raíz (0) no es un directorio")
		}
		return 0, inode, nil
	}

	// Verificar si el padre objetivo ya existe
	parentInodeIndex, parentInode, errFind := structures.FindInodeByPath(sb, partitionPath, targetParentPath)

	if errFind == nil { // Padre encontrado
		// Verificar si es un directorio
		if parentInode.I_type[0] != '0' {
			return -1, nil, fmt.Errorf("error: el path padre '%s' existe pero no es un directorio", targetParentPath)
		}
		// Padre existe y es directorio, todo bien
		fmt.Printf("Directorio padre '%s' (inodo %d) encontrado.\n", targetParentPath, parentInodeIndex)
		return parentInodeIndex, parentInode, nil
	}

	// Padre no encontrado
	fmt.Printf("Padre '%s' no encontrado (%v).\n", targetParentPath, errFind)
	if !createRecursively {
		// Si no es recursivo, fallamos
		return -1, nil, fmt.Errorf("el directorio padre '%s' no existe y la opción -r no fue especificada", targetParentPath)
	}

	//Intentar crear el padre
	grandParentPath := filepath.Dir(targetParentPath)
	parentDirName := filepath.Base(targetParentPath)

	_, _, errEnsureGrandParent := ensureParentDirExists(grandParentPath, true, sb, partitionPath) // Llamada recursiva
	if errEnsureGrandParent != nil {
		// Si falla crear el abuelo, no podemos crear el padre
		return -1, nil, fmt.Errorf("error asegurando ancestro '%s': %w", grandParentPath, errEnsureGrandParent)
	}

	// Ahora que el abuelo, creamos el padre
	fmt.Printf("Creando directorio padre faltante: '%s' dentro de '%s'\n", parentDirName, grandParentPath)
	parentDirsForCreate, destDirForCreate := utils.GetParentDirectories(targetParentPath)
	errCreate := sb.CreateFolder(partitionPath, parentDirsForCreate, destDirForCreate)
	if errCreate != nil {
		return -1, nil, fmt.Errorf("falló la creación recursiva del directorio padre '%s': %w", targetParentPath, errCreate)
	}

	// Si llegamos aquí, buscamos de nuevo el padre recién creado
	fmt.Printf("Verificando padre recién creado '%s'\n", targetParentPath)
	parentInodeIndex, parentInode, errFindAgain := structures.FindInodeByPath(sb, partitionPath, targetParentPath)
	if errFindAgain != nil {
		return -1, nil, fmt.Errorf("error crítico: no se encontró el directorio padre '%s' después de crearlo: %w", targetParentPath, errFindAgain)
	}
	if parentInode.I_type[0] != '0' {
		return -1, nil, fmt.Errorf("error crítico: el directorio padre '%s' recién creado no es un directorio", targetParentPath)
	}

	fmt.Printf("Directorio padre '%s' (inodo %d) creado y verificado.\n", targetParentPath, parentInodeIndex)
	return parentInodeIndex, parentInode, nil
}

// Retorna si existe, el índice del inodo encontrado y su tipo
func findEntryInParent(parentInode *structures.Inode, entryName string, sb *structures.SuperBlock, partitionPath string) (exists bool, foundInodeIndex int32, foundInodeType byte) {
	exists = false
	foundInodeIndex = -1
	foundInodeType = '?'

	if parentInode.I_type[0] != '0' {
		return
	}

	for _, blockPtr := range parentInode.I_block {
		if blockPtr == -1 {
			continue
		}
		if blockPtr < 0 || blockPtr >= sb.S_blocks_count {
			continue
		}

		folderBlock := &structures.FolderBlock{}
		offset := int64(sb.S_block_start) + int64(blockPtr)*int64(sb.S_block_size)
		if err := folderBlock.Deserialize(partitionPath, offset); err != nil {
			fmt.Printf("Advertencia: No se pudo leer el bloque de directorio %d al buscar '%s'\n", blockPtr, entryName)
			continue
		}

		for _, content := range folderBlock.B_content {
			if content.B_inodo != -1 {
				name := strings.TrimRight(string(content.B_name[:]), "\x00")
				if name == entryName {
					exists = true
					foundInodeIndex = content.B_inodo
					tempInode := &structures.Inode{}
					tempOffset := int64(sb.S_inode_start) + int64(foundInodeIndex)*int64(sb.S_inode_size)
					if err := tempInode.Deserialize(partitionPath, tempOffset); err == nil {
						foundInodeType = tempInode.I_type[0]
					}
					return
				}
			}
		}
	}
	return
}


func addEntryToParent(parentInodeIndex int32, entryName string, entryInodeIndex int32, sb *structures.SuperBlock, partitionPath string) error {

	parentInode := &structures.Inode{}
	parentOffset := int64(sb.S_inode_start) + int64(parentInodeIndex)*int64(sb.S_inode_size)
	if err := parentInode.Deserialize(partitionPath, parentOffset); err != nil {
		return fmt.Errorf("no se pudo leer inodo padre %d para añadir entrada: %w", parentInodeIndex, err)
	}
	if parentInode.I_type[0] != '0' {
		return fmt.Errorf("el inodo padre %d no es un directorio", parentInodeIndex)
	}

	fmt.Printf("Buscando slot libre en bloques existentes del padre %d...\n", parentInodeIndex)

	findAndAddInFolderBlock := func(blockPtr int32) (bool, error) {
		if blockPtr == -1 {
			return false, nil
		} 
		if blockPtr < 0 || blockPtr >= sb.S_blocks_count {
			fmt.Printf("Advertencia: Puntero inválido %d encontrado al buscar slot libre.\n", blockPtr)
			return false, nil 
		}

		folderBlock := &structures.FolderBlock{}
		blockOffset := int64(sb.S_block_start) + int64(blockPtr)*int64(sb.S_block_size)
		if err := folderBlock.Deserialize(partitionPath, blockOffset); err != nil {
			fmt.Printf("Advertencia: No se pudo leer bloque %d del padre %d para añadir entrada: %v\n", blockPtr, parentInodeIndex, err)
			return false, nil
		}

		for i := 0; i < len(folderBlock.B_content); i++ {
			if folderBlock.B_content[i].B_inodo == -1 { // Slot libre encontrado
				fmt.Printf("Encontrado slot libre %d en bloque existente %d del padre %d\n", i, blockPtr, parentInodeIndex)
				folderBlock.B_content[i].B_inodo = entryInodeIndex
				var cleanName [12]byte
				copy(cleanName[:], entryName)
				folderBlock.B_content[i].B_name = cleanName

				if err := folderBlock.Serialize(partitionPath, blockOffset); err != nil { // Serializar bloque modificado
					return false, fmt.Errorf("falló al escribir la nueva entrada en el bloque existente %d: %w", blockPtr, err)
				}
				// Actualizar tiempos del padre y serializar padre
				currentTime := float32(time.Now().Unix())
				parentInode.I_mtime = currentTime
				parentInode.I_atime = currentTime 
				if err := parentInode.Serialize(partitionPath, parentOffset); err != nil {
					fmt.Printf("Advertencia: falló al actualizar tiempos del inodo padre %d tras añadir en bloque %d: %v\n", parentInodeIndex, blockPtr, err)
				}
				return true, nil // Slot encontrado y usado
			}
		}
		return false, nil 
	} 

	// Buscar en bloques directos
	for k := 0; k < 12; k++ {
		found, err := findAndAddInFolderBlock(parentInode.I_block[k])
		if err != nil {
			return err
		}
		if found {
			return nil
		} 
	}

	if parentInode.I_block[12] != -1 {
		fmt.Printf("Buscando slot libre en bloques de indirección simple (L1 en %d)...\n", parentInode.I_block[12])
		l1Block := &structures.PointerBlock{}
		l1BlockIndex := parentInode.I_block[12]
		// Validar índice L1
		if l1BlockIndex < 0 || l1BlockIndex >= sb.S_blocks_count {
			fmt.Printf("Advertencia: Puntero indirecto simple (I_block[12]) inválido: %d\n", l1BlockIndex)
		} else {
			l1Offset := int64(sb.S_block_start) + int64(l1BlockIndex)*int64(sb.S_block_size)
			if err := l1Block.Deserialize(partitionPath, l1Offset); err == nil {
				for _, folderBlockPtr := range l1Block.P_pointers { // Iterar punteros L2 (que apuntan a FolderBlocks)
					found, err := findAndAddInFolderBlock(folderBlockPtr)
					if err != nil {
						return err
					}
					if found {
						return nil
					}
				}
			} else {
				fmt.Printf("Advertencia: No se pudo leer el bloque de punteros L1 %d: %v\n", l1BlockIndex, err)
			}
		}
	}
	//  Si no se encontró slot, asignar NUEVO bloque al padre 
	fmt.Printf("No se encontró slot libre en bloques existentes del padre %d. Buscando puntero libre para nuevo bloque...\n", parentInodeIndex)

	// --- Función auxiliar interna CORREGIDA ---
	allocateAndPrepareNewFolderBlock := func() (int32, *structures.FolderBlock, error) {
		if sb.S_free_blocks_count < 1 {
			return -1, nil, errors.New("no hay bloques libres para expandir directorio")
		}

		newBlockIndex, err := sb.FindFreeBlock(partitionPath)
		if err != nil {
			return -1, nil, fmt.Errorf("error al buscar bloque libre para expandir dir: %w", err)
		}
		// FindFreeBlock ya valida el índice devuelto

		// Actualizar bitmap y SB
		err = sb.UpdateBitmapBlock(partitionPath, newBlockIndex,'1')
		if err != nil {
			return -1, nil, fmt.Errorf("error bitmap para nuevo bloque dir %d: %w", newBlockIndex, err)
		}
		sb.S_free_blocks_count--

		// Crear, inicializar y serializar bloque vacío
		newFolderBlock := &structures.FolderBlock{}
		newFolderBlock.Initialize() // Usar el método Initialize

		newBlockOffset := int64(sb.S_block_start) + int64(newBlockIndex)*int64(sb.S_block_size)
		if err := newFolderBlock.Serialize(partitionPath, newBlockOffset); err != nil {
			return -1, nil, fmt.Errorf("falló al inicializar/serializar nuevo bloque dir %d: %w", newBlockIndex, err)
		}
		fmt.Printf("Nuevo bloque carpeta vacío asignado y serializado en índice %d\n", newBlockIndex)
		return newBlockIndex, newFolderBlock, nil 
	} 

	// Buscar puntero libre en bloques directos
	for k := 0; k < 12; k++ {
		if parentInode.I_block[k] == -1 {
			fmt.Printf("Encontrado puntero directo libre en I_block[%d]. Asignando nuevo bloque carpeta...\n", k)
			newBlockIndex, newFolderBlock, err := allocateAndPrepareNewFolderBlock()
			if err != nil {
				return err
			}
			// Actualizar inodo padre para apuntar al nuevo bloque
			parentInode.I_block[k] = newBlockIndex
			currentTime := float32(time.Now().Unix())
			parentInode.I_mtime = currentTime
			parentInode.I_atime = currentTime
			if err := parentInode.Serialize(partitionPath, parentOffset); err != nil {
				return fmt.Errorf("falló al actualizar I_block[%d] del padre %d: %w", k, parentInodeIndex, err)
			}

			// Añadir entrada al nuevo bloque (que está en memoria via newFolderBlock)
			newFolderBlock.B_content[0].B_inodo = entryInodeIndex // Usar índice 0
			var cleanName [12]byte
			copy(cleanName[:], entryName)
			newFolderBlock.B_content[0].B_name = cleanName

			newBlockOffset := int64(sb.S_block_start) + int64(newBlockIndex)*int64(sb.S_block_size)
			if err := newFolderBlock.Serialize(partitionPath, newBlockOffset); err != nil { // Sobrescribir con la entrada añadida
				return fmt.Errorf("falló al serializar nuevo bloque dir %d con la entrada: %w", newBlockIndex, err)
			}
			fmt.Printf("Nueva entrada '%s' -> %d añadida al nuevo bloque %d vía puntero directo.\n", entryName, entryInodeIndex, newBlockIndex)
			return nil // Éxito
		}
	}

	// Buscar/Crear puntero en indirecto simple (L1)
	fmt.Println("Punteros directos llenos. Verificando/Creando indirección simple (I_block[12])...")
	l1Ptr := parentInode.I_block[12]
	var l1Block *structures.PointerBlock 
	var l1BlockIndex int32               

	if l1Ptr == -1 { // Necesitamos crear el bloque L1
		fmt.Println("I_block[12] no existe. Creando bloque de punteros L1...")
		// Necesitamos 2 bloques: uno para L1 y otro para el FolderBlock
		if sb.S_free_blocks_count < 2 {
			return errors.New("no hay suficientes bloques libres para crear bloque L1 y bloque de carpeta")
		}
		// Asignar bloque L1
		l1Index, err := sb.FindFreeBlock(partitionPath)
		if err != nil {
			return fmt.Errorf("error buscando bloque para L1: %w", err)
		}
		if err = sb.UpdateBitmapBlock(partitionPath, l1Index,'1'); err != nil {
			return fmt.Errorf("error bitmap L1 %d: %w", l1Index, err)
		}
		sb.S_free_blocks_count--
		l1BlockIndex = l1Index

		// Actualizar inodo padre y serializarlo
		parentInode.I_block[12] = l1BlockIndex
		currentTime := float32(time.Now().Unix())
		parentInode.I_mtime = currentTime
		parentInode.I_atime = currentTime
		if err := parentInode.Serialize(partitionPath, parentOffset); err != nil {
			return fmt.Errorf("falló al actualizar I_block[12] del padre %d: %w", parentInodeIndex, err)
		}
		l1Block = &structures.PointerBlock{}
		for i := range l1Block.P_pointers {
			l1Block.P_pointers[i] = -1
		}
		l1Offset := int64(sb.S_block_start) + int64(l1BlockIndex)*int64(sb.S_block_size)
		if err := l1Block.Serialize(partitionPath, l1Offset); err != nil {
			return fmt.Errorf("falló al inicializar/serializar bloque L1 %d: %w", l1BlockIndex, err)
		}
		fmt.Printf("Bloque punteros L1 creado en índice %d\n", l1BlockIndex)
	} else {
		l1BlockIndex = l1Ptr
		if l1BlockIndex < 0 || l1BlockIndex >= sb.S_blocks_count {
			return fmt.Errorf("puntero indirecto simple (I_block[12]) inválido: %d", l1BlockIndex)
		}
		fmt.Printf("Bloque punteros L1 ya existe en índice %d. Cargando...\n", l1BlockIndex)
		l1Block = &structures.PointerBlock{}
		l1Offset := int64(sb.S_block_start) + int64(l1BlockIndex)*int64(sb.S_block_size)
		if err := l1Block.Deserialize(partitionPath, l1Offset); err != nil {
			return fmt.Errorf("no se pudo leer bloque de punteros L1 %d existente: %w", l1BlockIndex, err)
		}
	}

	foundL1PointerSlot := -1
	for k := 0; k < len(l1Block.P_pointers); k++ {
		if l1Block.P_pointers[k] == -1 {
			foundL1PointerSlot = k
			break
		}
	}

	if foundL1PointerSlot != -1 {
		fmt.Printf("Encontrado puntero libre en L1[%d]. Asignando nuevo bloque carpeta...\n", foundL1PointerSlot)
		// Asignar el nuevo bloque carpeta 
		newBlockIndex, newFolderBlock, err := allocateAndPrepareNewFolderBlock()
		if err != nil {
			return err
		}

		// Actualizar el bloque L1 para que apunte al nuevo bloque
		l1Block.P_pointers[foundL1PointerSlot] = newBlockIndex
		l1Offset := int64(sb.S_block_start) + int64(l1BlockIndex)*int64(sb.S_block_size)
		if err := l1Block.Serialize(partitionPath, l1Offset); err != nil {
			return fmt.Errorf("falló al serializar bloque puntero L1 %d actualizado: %w", l1BlockIndex, err)
		}

		// Añadir entrada al nuevo bloque carpeta (que está en memoria)
		newFolderBlock.B_content[0].B_inodo = entryInodeIndex // Usar slot 0
		var cleanName [12]byte
		copy(cleanName[:], entryName)
		newFolderBlock.B_content[0].B_name = cleanName

		// Serializar el nuevo bloque carpeta con la entrada
		newBlockOffset := int64(sb.S_block_start) + int64(newBlockIndex)*int64(sb.S_block_size)
		if err := newFolderBlock.Serialize(partitionPath, newBlockOffset); err != nil {
			return fmt.Errorf("falló al serializar nuevo bloque dir %d con la entrada: %w", newBlockIndex, err)
		}
		fmt.Printf("Nueva entrada '%s' -> %d añadida al nuevo bloque %d vía puntero indirecto simple.\n", entryName, entryInodeIndex, newBlockIndex)
		return nil
	}
	return fmt.Errorf("directorio padre (inodo %d) lleno: no hay espacio en bloques existentes ni en punteros directos/indirectos simples. Indirección doble/triple no implementada para directorios", parentInodeIndex)
}

// allocateDataBlocks asigna bloques de datos para un archivo, actualizando el superbloque y el bitmap.1
func allocateDataBlocks(contentBytes []byte, fileSize int32, sb *structures.SuperBlock, partitionPath string) ([15]int32, error) {
	allocatedBlockIndices := [15]int32{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}

	if fileSize == 0 {
		return allocatedBlockIndices, nil
	}

	blockSize := sb.S_block_size
	if blockSize <= 0 {
		return allocatedBlockIndices, errors.New("tamaño de bloque inválido en superbloque al asignar bloques")
	}
	numBlocksNeeded := (fileSize + blockSize - 1) / blockSize

	fmt.Printf("Allocate: Necesitando %d bloques para %d bytes (tamaño bloque: %d)\n", numBlocksNeeded, fileSize, blockSize)

	directLimit := int32(12)
	pointersPerBlock := int32(len(structures.PointerBlock{}.P_pointers))
	if pointersPerBlock <= 0 {
		return allocatedBlockIndices, errors.New("cálculo inválido de punteros por bloque")
	}
	simpleLimit := directLimit + pointersPerBlock
	doubleLimit := simpleLimit + pointersPerBlock*pointersPerBlock
	tripleLimit := doubleLimit + pointersPerBlock*pointersPerBlock*pointersPerBlock

	if numBlocksNeeded > tripleLimit {
		return allocatedBlockIndices, fmt.Errorf("el archivo es demasiado grande (%d bloques), excede el límite de indirección triple (%d bloques)", numBlocksNeeded, tripleLimit)
	}
	if numBlocksNeeded > sb.S_free_blocks_count {
		return allocatedBlockIndices, fmt.Errorf("espacio insuficiente: se necesitan %d bloques, disponibles %d", numBlocksNeeded, sb.S_free_blocks_count)
	}

	// Variables para bloques indirectos
	var indirect1Block *structures.PointerBlock = nil // Simple L1
	var indirect1BlockIndex int32 = -1
	var indirect2Blocks [16]*structures.PointerBlock = [16]*structures.PointerBlock{}
	var indirect2BlockIndices [16]int32 = [16]int32{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
	var indirect2L1Block *structures.PointerBlock = nil
	var indirect2L1BlockIndex int32 = -1

	for b := int32(0); b < numBlocksNeeded; b++ {
		dataBlockIndex, err := sb.FindFreeBlock(partitionPath)
		if err != nil {
			return allocatedBlockIndices, fmt.Errorf("no se encontró bloque de datos libre (necesitaba el bloque #%d de %d): %w", b+1, numBlocksNeeded, err)
		}

		fmt.Printf("Allocate: Bloque libre encontrado: %d (para bloque de datos #%d)\n", dataBlockIndex, b)

		// Actualizar bitmap y SB para el bloque de DATOS
		err = sb.UpdateBitmapBlock(partitionPath, dataBlockIndex,'1')
		if err != nil {
			return allocatedBlockIndices, fmt.Errorf("error bitmap bloque datos %d: %w", dataBlockIndex, err)
		}
		sb.S_free_blocks_count-- // Decrementar contador

		// Escribir datos en el bloque
		fileBlock := &structures.FileBlock{}
		start := b * blockSize
		end := start + blockSize
		if end > fileSize {
			end = fileSize
		}
		bytesToWrite := contentBytes[start:end]
		copy(fileBlock.B_content[:], bytesToWrite) // Copia hasta 64 bytes
		blockOffset := int64(sb.S_block_start) + int64(dataBlockIndex)*int64(sb.S_block_size)
		err = fileBlock.Serialize(partitionPath, blockOffset)
		if err != nil {
			return allocatedBlockIndices, fmt.Errorf("error serializando bloque datos %d: %w", dataBlockIndex, err)
		}

		// Directos (0-11)
		if b < directLimit {
			allocatedBlockIndices[b] = dataBlockIndex
			fmt.Printf("Allocate: Bloque datos %d asignado a I_block[%d]\n", dataBlockIndex, b)
			continue
		}

		// Indirecto Simple (12 hasta simpleLimit-1)
		if b < simpleLimit {
			idxInSimple := b - directLimit // Índice dentro del bloque de punteros simple (0 a pointersPerBlock-1)
			fmt.Printf("Allocate: Bloque datos %d necesita ir en Indirecto Simple (idx %d)\n", dataBlockIndex, idxInSimple)

			// Asignar el bloque de punteros L1 (Simple) si es la primera vez
			if indirect1Block == nil {
				fmt.Println("Allocate: Asignando Bloque Punteros L1 (Simple)...")
				indirect1BlockIndex, err = sb.FindFreeBlock(partitionPath)
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("no se pudo asignar bloque para punteros L1 (Simple): %w", err)
				}

				err = sb.UpdateBitmapBlock(partitionPath, indirect1BlockIndex,'1')
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("error bitmap bloque punteros L1 %d: %w", indirect1BlockIndex, err)
				}
				sb.S_free_blocks_count--

				allocatedBlockIndices[12] = indirect1BlockIndex
				indirect1Block = &structures.PointerBlock{}   
				for i := range indirect1Block.P_pointers {    
					indirect1Block.P_pointers[i] = -1
				}
				// Serializar el bloque L1 vacío AHORA
				offsetL1 := int64(sb.S_block_start) + int64(indirect1BlockIndex)*int64(sb.S_block_size)
				err = indirect1Block.Serialize(partitionPath, offsetL1)
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("error serializando bloque puntero L1 simple INICIAL %d: %w", indirect1BlockIndex, err)
				}
				fmt.Printf("Allocate: Bloque Punteros L1 (Simple) asignado al índice %d y serializado vacío\n", indirect1BlockIndex)
			}
			// Guardar puntero al bloque de datos en el struct del bloque de punteros L1
			indirect1Block.P_pointers[idxInSimple] = dataBlockIndex
			fmt.Printf("Allocate: Puntero a datos %d guardado en P_pointers[%d] del Bloque L1 (Simple) en memoria\n", dataBlockIndex, idxInSimple)
			continue
		}

		// Indirecto Doble (simpleLimit hasta doubleLimit-1)
		if b < doubleLimit {
			relIdxDouble := b - simpleLimit     
			idxL1 := relIdxDouble / pointersPerBlock 
			idxL2 := relIdxDouble % pointersPerBlock 
			fmt.Printf("Allocate: Bloque datos %d necesita ir en Indirecto Doble (L1[%d], L2[%d])\n", dataBlockIndex, idxL1, idxL2)

			// Asignar el bloque de punteros L1 (Doble) si es la primera vez
			if indirect2L1Block == nil {
				fmt.Println("Allocate: Asignando Bloque Punteros L1 (Doble)...")
				indirect2L1BlockIndex, err = sb.FindFreeBlock(partitionPath)
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("no se pudo asignar bloque para punteros L1 (Doble): %w", err)
				}

				err = sb.UpdateBitmapBlock(partitionPath, indirect2L1BlockIndex,'1')
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("error bitmap bloque punteros L1 doble %d: %w", indirect2L1BlockIndex, err)
				}
				sb.S_free_blocks_count--

				allocatedBlockIndices[13] = indirect2L1BlockIndex 
				indirect2L1Block = &structures.PointerBlock{}
				for i := range indirect2L1Block.P_pointers {
					indirect2L1Block.P_pointers[i] = -1
				}
				// Serializar L1 doble vacío
				offsetL1D := int64(sb.S_block_start) + int64(indirect2L1BlockIndex)*int64(sb.S_block_size)
				err = indirect2L1Block.Serialize(partitionPath, offsetL1D)
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("error serializando bloque puntero L1 doble INICIAL %d: %w", indirect2L1BlockIndex, err)
				}
				fmt.Printf("Allocate: Bloque Punteros L1 (Doble) asignado al índice %d y serializado vacío\n", indirect2L1BlockIndex)
			}

			// Asignar el bloque de punteros L2 si es la primera vez para este índice L1
			if indirect2Blocks[idxL1] == nil {
				fmt.Printf("Allocate: Asignando Bloque Punteros L2 (para L1[%d])...\n", idxL1)
				blockIndexL2, err := sb.FindFreeBlock(partitionPath)
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("no se pudo asignar bloque para punteros L2 (idxL1=%d): %w", idxL1, err)
				}

				err = sb.UpdateBitmapBlock(partitionPath, blockIndexL2,'1')
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("error bitmap bloque punteros L2 %d: %w", blockIndexL2, err)
				}
				sb.S_free_blocks_count--

				indirect2L1Block.P_pointers[idxL1] = blockIndexL2   // Guardar puntero a L2 en L1 (en memoria)
				indirect2Blocks[idxL1] = &structures.PointerBlock{} // Crear struct L2 en memoria
				indirect2BlockIndices[idxL1] = blockIndexL2         // Guardar índice L2 para serialización posterior
				for i := range indirect2Blocks[idxL1].P_pointers {
					indirect2Blocks[idxL1].P_pointers[i] = -1
				}
				// Serializar L2 vacío ahora
				offsetL2 := int64(sb.S_block_start) + int64(blockIndexL2)*int64(sb.S_block_size)
				err = indirect2Blocks[idxL1].Serialize(partitionPath, offsetL2)
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("error serializando bloque puntero L2 INICIAL %d: %w", blockIndexL2, err)
				}

				fmt.Printf("Allocate: Bloque Punteros L2 asignado al índice %d (guardado en L1[%d]) y serializado vacío\n", blockIndexL2, idxL1)

				// Serializar L1 AHORA porque cambió su puntero a L2
				offsetL1 := int64(sb.S_block_start) + int64(indirect2L1BlockIndex)*int64(sb.S_block_size)
				err = indirect2L1Block.Serialize(partitionPath, offsetL1)
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("error serializando bloque puntero L1 doble %d tras añadir puntero a L2: %w", indirect2L1BlockIndex, err)
				}
			}

			// Guardar puntero al bloque de datos en el struct del bloque de punteros L2 correspondiente (en memoria)
			indirect2Blocks[idxL1].P_pointers[idxL2] = dataBlockIndex
			fmt.Printf("Allocate: Puntero a datos %d guardado en P_pointers[%d] del Bloque L2 (índice %d) en memoria\n", dataBlockIndex, idxL2, indirect2BlockIndices[idxL1])
			continue
		}

		//No lo haré xd
		if b < tripleLimit {
			return allocatedBlockIndices, fmt.Errorf("la indirección triple (bloque %d) no está implementada", b)
		}
	}

	// Serializar Bloques de Punteros Pendientes
	if indirect1Block != nil {
		fmt.Printf("Allocate: Serializando Bloque Punteros L1 (Simple) final %d\n", indirect1BlockIndex)
		offset := int64(sb.S_block_start) + int64(indirect1BlockIndex)*int64(sb.S_block_size)
		err := indirect1Block.Serialize(partitionPath, offset)
		if err != nil {
			return allocatedBlockIndices, fmt.Errorf("error serializando bloque puntero L1 simple final %d: %w", indirect1BlockIndex, err)
		}
	}
	// Serializar L2 para Doble
	if indirect2L1Block != nil { // Si se usó doble indirección
		for idxL1 := 0; idxL1 < len(indirect2Blocks); idxL1++ {
			if indirect2Blocks[idxL1] != nil { // Si este bloque L2 fue creado/usado
				idxL2Block := indirect2BlockIndices[idxL1] 
				fmt.Printf("Allocate: Serializando Bloque Punteros L2 final %d (desde L1[%d])\n", idxL2Block, idxL1)
				offsetL2 := int64(sb.S_block_start) + int64(idxL2Block)*int64(sb.S_block_size)
				err := indirect2Blocks[idxL1].Serialize(partitionPath, offsetL2)
				if err != nil {
					return allocatedBlockIndices, fmt.Errorf("error serializando bloque puntero L2 final %d: %w", idxL2Block, err)
				}
			}
		}
	}

	fmt.Println("Allocate: Asignación de bloques de datos completada.")
	return allocatedBlockIndices, nil
}
