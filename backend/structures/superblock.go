package structures

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type SuperBlock struct {
	S_filesystem_type   int32
	S_inodes_count      int32
	S_blocks_count      int32
	S_free_inodes_count int32
	S_free_blocks_count int32
	S_mtime             float32
	S_umtime            float32
	S_mnt_count         int32
	S_magic             int32
	S_inode_size        int32
	S_block_size        int32
	S_first_ino         int32
	S_first_blo         int32
	S_bm_inode_start    int32
	S_bm_block_start    int32
	S_inode_start       int32
	S_block_start       int32
	// Total: 68 bytes
}

func (sb *SuperBlock) Serialize(path string, offset int64) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Mover el puntero del archivo a la posición especificada
	_, err = file.Seek(offset, 0)
	if err != nil {
		return err
	}

	// Serializar la estructura SuperBlock directamente en el archivo
	err = binary.Write(file, binary.LittleEndian, sb)
	if err != nil {
		return err
	}

	return nil
}

func (sb *SuperBlock) Deserialize(path string, offset int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Mover el puntero del archivo a la posición especificada
	_, err = file.Seek(offset, 0)
	if err != nil {
		return err
	}

	// Obtener el tamaño de la estructura SuperBlock
	sbSize := binary.Size(sb)
	if sbSize <= 0 {
		return fmt.Errorf("invalid SuperBlock size: %d", sbSize)
	}

	// Leer solo la cantidad de bytes que corresponden al tamaño de la estructura SuperBlock
	buffer := make([]byte, sbSize)
	_, err = file.Read(buffer)
	if err != nil {
		return err
	}

	// Deserializar los bytes leídos en la estructura SuperBlock
	reader := bytes.NewReader(buffer)
	err = binary.Read(reader, binary.LittleEndian, sb)
	if err != nil {
		return err
	}

	return nil
}

func (sb *SuperBlock) Print() {
	mountTime := time.Unix(int64(sb.S_mtime), 0)
	unmountTime := time.Unix(int64(sb.S_umtime), 0)

	fmt.Printf("Filesystem Type: %d\n", sb.S_filesystem_type)
	fmt.Printf("Inodes Count: %d\n", sb.S_inodes_count)
	fmt.Printf("Blocks Count: %d\n", sb.S_blocks_count)
	fmt.Printf("Free Inodes Count: %d\n", sb.S_free_inodes_count)
	fmt.Printf("Free Blocks Count: %d\n", sb.S_free_blocks_count)
	fmt.Printf("Mount Time: %s\n", mountTime.Format(time.RFC3339))
	fmt.Printf("Unmount Time: %s\n", unmountTime.Format(time.RFC3339))
	fmt.Printf("Mount Count: %d\n", sb.S_mnt_count)
	fmt.Printf("Magic: %d\n", sb.S_magic)
	fmt.Printf("Inode Size: %d\n", sb.S_inode_size)
	fmt.Printf("Block Size: %d\n", sb.S_block_size)
	fmt.Printf("First Inode: %d\n", sb.S_first_ino)
	fmt.Printf("First Block: %d\n", sb.S_first_blo)
	fmt.Printf("Bitmap Inode Start: %d\n", sb.S_bm_inode_start)
	fmt.Printf("Bitmap Block Start: %d\n", sb.S_bm_block_start)
	fmt.Printf("Inode Start: %d\n", sb.S_inode_start)
	fmt.Printf("Block Start: %d\n", sb.S_block_start)
}

func (sb *SuperBlock) PrintInodes(path string) error {
	fmt.Println("\nInodos\n----------------")
	for i := int32(0); i < sb.S_inodes_count; i++ {
		inode := &Inode{}
		err := inode.Deserialize(path, int64(sb.S_inode_start+(i*sb.S_inode_size)))
		if err != nil {
			return err
		}
		fmt.Printf("\nInodo %d:\n", i)
		inode.Print()
	}

	return nil
}

func (sb *SuperBlock) PrintBlocks(path string) error {
	fmt.Println("\nBloques\n----------------")
	visitedBlocks := make(map[int32]bool) 

	for i := int32(0); i < sb.S_inodes_count; i++ {
		inode := &Inode{}
		offset := int64(sb.S_inode_start + (i * sb.S_inode_size))
		err := inode.Deserialize(path, offset)
		if err != nil {
			continue 
		}

		isUsed, _ := sb.IsInodeUsed(path, i)
		if !isUsed {
			continue 
		}

		var blocksToProcess []int32
		// Añadir bloques directos
		for j := 0; j < 12; j++ {
			if inode.I_block[j] != -1 {
				blocksToProcess = append(blocksToProcess, inode.I_block[j])
			}
		}

		for _, blockIndex := range blocksToProcess {
			if blockIndex == -1 || visitedBlocks[blockIndex] {
				continue // Saltar bloques inválidos o ya visitados
			}
			visitedBlocks[blockIndex] = true

			blockOffset := int64(sb.S_block_start + (blockIndex * sb.S_block_size))

			isBlockUsed, _ := sb.IsBlockUsed(path, blockIndex)
			status := "Libre"
			if isBlockUsed {
				status = "Usado"
			}

			fmt.Printf("\nBloque %d (Offset: %d, Status: %s):\n", blockIndex, blockOffset, status)

			if inode.I_type[0] == '0' { // Bloque de Carpeta
				block := &FolderBlock{}
				err := block.Deserialize(path, blockOffset)
				if err != nil {
					fmt.Printf("  Error al leer como FolderBlock: %v\n", err)
				} else {
					fmt.Println("  Tipo: FolderBlock")
					block.Print() 
				}
			} else if inode.I_type[0] == '1' { // Bloque de Archivo
				block := &FileBlock{}
				err := block.Deserialize(path, blockOffset)
				if err != nil {
					fmt.Printf("  Error al leer como FileBlock: %v\n", err)
				} else {
					fmt.Println("  Tipo: FileBlock")
					block.Print() 
				}
			} else {
				fmt.Println("  Tipo: Desconocido o Bloque de Punteros (Indirecto)")
			}
		}
	}
	return nil
}

func (sb *SuperBlock) CreateFolder(diskPath string, parentsDir []string, destDir string) error {
	fmt.Printf(">> CreateFolder: diskPath='%s', parentsDir=%v, destDir='%s'\n", diskPath, parentsDir, destDir)

	// Encontrar el inodo del directorio padre
	parentPath := "/" + strings.Join(parentsDir, "/")
	if !strings.HasPrefix(parentPath, "//") && parentPath != "/" {
			parentPath = strings.TrimSuffix(parentPath, "/") // Asegurar que no termine en / a menos que sea la raíz
	} else if parentPath == "//" {
		parentPath = "/" // Corregir doble slash inicial
	}

	fmt.Printf("   Buscando inodo padre para: '%s'\n", parentPath)
	parentInodeNum, parentInode, err := FindInodeByPath(sb, diskPath, parentPath)
	if err != nil {
		return fmt.Errorf("error crítico: no se encontró el inodo padre '%s' (%w)", parentPath, err)
	}
	if parentInode.I_type[0] != '0' {
		return fmt.Errorf("error: la ruta padre '%s' no es un directorio", parentPath)
	}

	fmt.Printf("   Inodo padre encontrado: %d\n", parentInodeNum)
	parentInodeOffset := int64(sb.S_inode_start + parentInodeNum*sb.S_inode_size)

	//Verificar si destDir ya existe en el padre
	entryExists := false
	for blockPtrIndex := 0; blockPtrIndex < 12; blockPtrIndex++ { 
		blockPtr := parentInode.I_block[blockPtrIndex]
		if blockPtr == -1 {
			continue
		}
		folderBlock := &FolderBlock{}
		blockOffset := int64(sb.S_block_start + blockPtr*sb.S_block_size)
		if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
			fmt.Printf("   Advertencia: Error leyendo bloque %d del padre: %v\n", blockPtr, err)
			continue
		}
		for _, entry := range folderBlock.B_content {
			if entry.B_inodo != -1 {
				name := strings.TrimRight(string(entry.B_name[:]), "\x00")
				if name == destDir {
					entryExists = true
					break
				}
			}
		}
		if entryExists {
			break
		}
	}
	if entryExists {
		// Si se usa -p, esto no es un error, simplemente ya existe.
		fmt.Printf("   Directorio '%s' ya existe en '%s'.\n", destDir, parentPath)
		return nil
	}

	// Buscar un slot libre en los bloques existentes del padre
	foundSlot := false
	var targetBlockPtr int32 = -1
	var targetEntryIndex int = -1

	fmt.Printf("   Buscando slot libre en bloques existentes del inodo padre %d...\n", parentInodeNum)
	for blockPtrIndex := 0; blockPtrIndex < 12; blockPtrIndex++ {
		blockPtr := parentInode.I_block[blockPtrIndex]
		if blockPtr == -1 {
			continue // Este puntero directo no está usado
		}
		fmt.Printf("      Examinando bloque directo %d (puntero %d)\n", blockPtrIndex, blockPtr)
		folderBlock := &FolderBlock{}
		blockOffset := int64(sb.S_block_start + blockPtr*sb.S_block_size)
		if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
			fmt.Printf("      Advertencia: Error leyendo bloque %d: %v. Saltando.\n", blockPtr, err)
			continue
		}
		for entryIndex, entry := range folderBlock.B_content {
			if entry.B_inodo == -1 { 
				fmt.Printf("      Slot libre encontrado, Bloque %d, Índice %d\n", blockPtr, entryIndex)
				foundSlot = true
				targetBlockPtr = blockPtr
				targetEntryIndex = entryIndex
				break
			}
		}
		if foundSlot {
			break
		}
	}

	// Si NO se encontró slot libre, asignar nuevo bloque al padre
	var parentModifiedBlock *FolderBlock // Guardará el bloque modificado
	var parentModifiedBlockOffset int64  // Offset del bloque modificado

	if !foundSlot {
		fmt.Printf("   No se encontró slot libre. Intentando asignar nuevo bloque al inodo padre %d...\n", parentInodeNum)
		freePtrIndex := -1
		for i := 0; i < 12; i++ {
			if parentInode.I_block[i] == -1 {
				freePtrIndex = i
				break
			}
		}
		if freePtrIndex == -1 {
			return errors.New("no hay punteros directos libres en el inodo padre")
		}
		fmt.Printf("      Puntero directo libre encontrado en I_block[%d]\n", freePtrIndex)

		// Asignar nuevo bloque del bitmap
		newParentBlockIndex, err := sb.FindFreeBlock(diskPath)
		if err != nil {
			return fmt.Errorf("no se pudo asignar un nuevo bloque para el directorio padre: %w", err)
		}
		fmt.Printf("      Asignando bloque libre %d del bitmap\n", newParentBlockIndex)

		// Actualizar bitmap de bloques
		if err := sb.UpdateBitmapBlock(diskPath, newParentBlockIndex); err != nil {
			return fmt.Errorf("error al actualizar bitmap de bloques para el nuevo bloque %d: %w", newParentBlockIndex, err)
		}
		sb.S_free_blocks_count-- // Decrementar contador global

		// Actualizar puntero en inodo padre
		parentInode.I_block[freePtrIndex] = newParentBlockIndex
		parentInode.I_mtime = float32(time.Now().Unix()) // Actualizar tiempo de modificación

		// Serializar inodo padre actualizado
		if err := parentInode.Serialize(diskPath, parentInodeOffset); err != nil {
			return fmt.Errorf("error al serializar inodo padre actualizado (offset %d): %w", parentInodeOffset, err)
		}
		fmt.Printf("      Inodo padre %d actualizado con puntero a nuevo bloque %d en índice %d\n", parentInodeNum, newParentBlockIndex, freePtrIndex)

		// Inicializar el nuevo bloque como FolderBlock vacío
		newParentBlock := &FolderBlock{}
		newParentBlock.Initialize() 
		parentModifiedBlock = newParentBlock
		parentModifiedBlockOffset = int64(sb.S_block_start + newParentBlockIndex*sb.S_block_size)

		// Usaremos el primer slot de este nuevo bloque
		targetBlockPtr = newParentBlockIndex
		targetEntryIndex = 0
		fmt.Printf("      Nuevo bloque %d inicializado. Se usará el slot 0.\n", newParentBlockIndex)

	} else {
		// Si sí encontramos slot, necesitamos leer ese bloque para modificarlo
		parentModifiedBlock = &FolderBlock{}
		parentModifiedBlockOffset = int64(sb.S_block_start + targetBlockPtr*sb.S_block_size)
		if err := parentModifiedBlock.Deserialize(diskPath, parentModifiedBlockOffset); err != nil {
			return fmt.Errorf("error al leer bloque padre %d para modificación: %w", targetBlockPtr, err)
		}
		fmt.Printf("   Se usará el slot %d del bloque existente %d.\n", targetEntryIndex, targetBlockPtr)
	}

	//Asignar nuevo INODO para el directorio a crear
	newDirInodeNum, err := sb.FindFreeInode(diskPath)
	if err != nil {
		return fmt.Errorf("no se pudo asignar un nuevo inodo para '%s': %w", destDir, err)
	}
	fmt.Printf("      Asignando inodo libre %d para '%s'\n", newDirInodeNum, destDir)
	if err := sb.UpdateBitmapInode(diskPath, newDirInodeNum); err != nil {
		return fmt.Errorf("error al actualizar bitmap de inodos para el nuevo inodo %d: %w", newDirInodeNum, err)
	}
	sb.S_free_inodes_count--
	newDirInodeOffset := int64(sb.S_inode_start + newDirInodeNum*sb.S_inode_size)

	// Asignar nuevo BLOQUE para el contenido de destDir
	newDirBlockIndex, err := sb.FindFreeBlock(diskPath)
	if err != nil {
		return fmt.Errorf("no se pudo asignar un nuevo bloque para '%s': %w", destDir, err)
	}
	fmt.Printf("      Asignando bloque libre %d para contenido de '%s'\n", newDirBlockIndex, destDir)
	if err := sb.UpdateBitmapBlock(diskPath, newDirBlockIndex); err != nil {
		return fmt.Errorf("error al actualizar bitmap de bloques para el bloque %d: %w", newDirBlockIndex, err)
	}
	sb.S_free_blocks_count--
	newDirBlockOffset := int64(sb.S_block_start + newDirBlockIndex*sb.S_block_size)

	// Inicializar el NUEVO INODO 
	newDirInode := &Inode{}
	now := float32(time.Now().Unix())
	newDirInode.I_uid = 0
	newDirInode.I_gid = 0
	newDirInode.I_size = 0 
	newDirInode.I_atime = now
	newDirInode.I_ctime = now
	newDirInode.I_mtime = now
	newDirInode.I_type[0] = '0' 
	copy(newDirInode.I_perm[:], "777") // Permiso por defecto
	for i := range newDirInode.I_block {
		newDirInode.I_block[i] = -1
	}
	// Asignar el primer bloque directo
	newDirInode.I_block[0] = newDirBlockIndex

	// Serializar el nuevo inodo
	if err := newDirInode.Serialize(diskPath, newDirInodeOffset); err != nil {
		return fmt.Errorf("error al serializar el nuevo inodo %d (offset %d): %w", newDirInodeNum, newDirInodeOffset, err)
	}
	fmt.Printf("      Nuevo inodo %d inicializado y serializado.\n", newDirInodeNum)

	// Inicializar el NUEVO BLOQUE 
	newDirFolderBlock := &FolderBlock{}
	newDirFolderBlock.Initialize()
	copy(newDirFolderBlock.B_content[0].B_name[:], ".")
	newDirFolderBlock.B_content[0].B_inodo = newDirInodeNum
	copy(newDirFolderBlock.B_content[1].B_name[:], "..")
	newDirFolderBlock.B_content[1].B_inodo = parentInodeNum

	// Serializar el nuevo bloque de carpeta
	if err := newDirFolderBlock.Serialize(diskPath, newDirBlockOffset); err != nil {
		return fmt.Errorf("error al serializar el nuevo bloque de carpeta %d (offset %d): %w", newDirBlockIndex, newDirBlockOffset, err)
	}
	fmt.Printf("      Nuevo bloque %d inicializado con '.' y '..' y serializado.\n", newDirBlockIndex)

	// Actualizar la entrada en el BLOQUE PADRE MODIFICADO
	copy(parentModifiedBlock.B_content[targetEntryIndex].B_name[:], destDir)
	parentModifiedBlock.B_content[targetEntryIndex].B_inodo = newDirInodeNum

	// Serializar el bloque padre modificado
	if err := parentModifiedBlock.Serialize(diskPath, parentModifiedBlockOffset); err != nil {
		return fmt.Errorf("error al serializar el bloque padre modificado %d (offset %d): %w", targetBlockPtr, parentModifiedBlockOffset, err)
	}
	fmt.Printf("      Bloque padre %d actualizado con entrada para '%s' -> %d y serializado.\n", targetBlockPtr, destDir, newDirInodeNum)

	// Actualizar tiempo del inodo padre (ya se hizo si se añadió bloque, hacerlo si no)
	if foundSlot { // Actualizar mtime ahora
		parentInode.I_mtime = float32(time.Now().Unix())
		if err := parentInode.Serialize(diskPath, parentInodeOffset); err != nil {
			fmt.Printf("   Advertencia: error al actualizar mtime del inodo padre %d: %v\n", parentInodeNum, err)
		}
	}
	fmt.Printf(">> Directorio '%s' creado exitosamente.\n", destDir)
	return nil
}

// PARA EL LOGIN --------------------------------------------------------------------------------------------------------------------

// Get users.txt block
func (sb *SuperBlock) GetUsersBlock(path string) (*FileBlock, error) {
	// Ir al inodo 1
	inode := &Inode{}

	// Deserializar el inodo
	err := inode.Deserialize(path, int64(sb.S_inode_start+(1*sb.S_inode_size)))
	if err != nil {
		return nil, err
	}

	// Iterar sobre cada bloque del inodo (apuntadores)
	for _, blockIndex := range inode.I_block {
		// Si el bloque no existe, salir
		if blockIndex == -1 {
			break
		}
		// Si el inodo es de tipo archivo
		if inode.I_type[0] == '1' {
			block := &FileBlock{}
			// Deserializar el bloque
			err := block.Deserialize(path, int64(sb.S_block_start+(blockIndex*sb.S_block_size))) // 64 porque es el tamaño de un bloque
			if err != nil {
				return nil, err
			}

			return block, nil
		}
	}
	return nil, fmt.Errorf("users.txt block not found")
}

// FindFreeInode busca el primer inodo libre ('0') en el bitmap de inodos.
func (sb *SuperBlock) FindFreeInode(diskPath string) (int32, error) {
	file, err := os.Open(diskPath)
	if err != nil {
		return -1, fmt.Errorf("error al abrir disco para buscar inodo libre: %w", err)
	}
	defer file.Close()

	if sb.S_inodes_count <= 0 || sb.S_inodes_count > 1000000 { // Límite arbitrario
		return -1, fmt.Errorf("número de inodos inválido o excesivo: %d", sb.S_inodes_count)
	}
	bitmap := make([]byte, sb.S_inodes_count)
	_, err = file.ReadAt(bitmap, int64(sb.S_bm_inode_start))
	if err != nil {
		return -1, fmt.Errorf("error al leer bitmap de inodos: %w", err)
	}

	// Buscar el primer byte '0'
	startIndex := sb.S_first_ino
	if startIndex < 0 || startIndex >= sb.S_inodes_count {
			startIndex = 0 // Empezar desde el principio si S_first_ino no es útil
	}

	for i := int(startIndex); i < len(bitmap); i++ {
		if bitmap[i] == '0' {
			sb.S_first_ino = int32(i) + 1
			return int32(i), nil
		}
	}
	if startIndex > 0 {
		for i := 0; i < int(startIndex); i++ {
			if bitmap[i] == '0' {
				sb.S_first_ino = int32(i) + 1
				return int32(i), nil
			}
		}
	}


	return -1, errors.New("no hay inodos libres disponibles")
}

// FindFreeBlock busca el primer bloque libre ('0') en el bitmap de bloques.
func (sb *SuperBlock) FindFreeBlock(diskPath string) (int32, error) {
	file, err := os.Open(diskPath)
	if err != nil {
		return -1, fmt.Errorf("error al abrir disco para buscar bloque libre: %w", err)
	}
	defer file.Close()

	// Buffer para leer el bitmap de bloques completo
	if sb.S_blocks_count <= 0 || sb.S_blocks_count > 10000000 { //Cualquier limite xd
		return -1, fmt.Errorf("número de bloques inválido o excesivo: %d", sb.S_blocks_count)
	}
	bitmap := make([]byte, sb.S_blocks_count)
	_, err = file.ReadAt(bitmap, int64(sb.S_bm_block_start))
	if err != nil {
		return -1, fmt.Errorf("error al leer bitmap de bloques: %w", err)
	}

	// Buscar el primer byte '0'
	startIndex := sb.S_first_blo
	if startIndex < 0 || startIndex >= sb.S_blocks_count {
			startIndex = 0
	}

	for i := int(startIndex); i < len(bitmap); i++ {
		if bitmap[i] == '0' {
			sb.S_first_blo = int32(i) + 1 
			return int32(i), nil
		}
	}
	// Buscar desde el principio si no se encontró
	if startIndex > 0 {
		for i := 0; i < int(startIndex); i++ {
			if bitmap[i] == '0' {
				sb.S_first_blo = int32(i) + 1
				return int32(i), nil
			}
		}
	}

	return -1, errors.New("no hay bloques libres disponibles")
}

// IsInodeUsed verifica el estado de un inodo en el bitmap.
func (sb *SuperBlock) IsInodeUsed(diskPath string, inodeIndex int32) (bool, error) {
    if inodeIndex < 0 || inodeIndex >= sb.S_inodes_count {
        return false, fmt.Errorf("índice de inodo fuera de rango: %d", inodeIndex)
    }
    file, err := os.Open(diskPath)
    if err != nil {
        return false, fmt.Errorf("error al abrir disco para verificar inodo: %w", err)
    }
    defer file.Close()

    inodeStatus := make([]byte, 1)
    offset := int64(sb.S_bm_inode_start) + int64(inodeIndex)
    _, err = file.ReadAt(inodeStatus, offset)
    if err != nil {
        return false, fmt.Errorf("error al leer estado del inodo %d en bitmap: %w", inodeIndex, err)
    }

    return inodeStatus[0] == '1', nil // '1' significa usado
}

// IsBlockUsed verifica el estado de un bloque en el bitmap.
func (sb *SuperBlock) IsBlockUsed(diskPath string, blockIndex int32) (bool, error) {
    if blockIndex < 0 || blockIndex >= sb.S_blocks_count {
        return false, fmt.Errorf("índice de bloque fuera de rango: %d", blockIndex)
    }
    file, err := os.Open(diskPath)
    if err != nil {
        return false, fmt.Errorf("error al abrir disco para verificar bloque: %w", err)
    }
    defer file.Close()

    blockStatus := make([]byte, 1)
    offset := int64(sb.S_bm_block_start) + int64(blockIndex)
    _, err = file.ReadAt(blockStatus, offset)
    if err != nil {
        return false, fmt.Errorf("error al leer estado del bloque %d en bitmap: %w", blockIndex, err)
    }

    return blockStatus[0] == '1', nil // usado
}







