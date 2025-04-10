package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"time"
)

type MKFS struct {
	id  string // ID del disco
	typ string // Tipo de formato
	fs  string // Tipo de sistema de archivos (ext2, ext3)
}

func ParseMkfs(tokens []string) (string, error) {
	cmd := &MKFS{}
	processedKeys := make(map[string]bool) // Para evitar duplicados

	idRegex := regexp.MustCompile(`^(?i)-id=(?:"([^"]+)"|([^\s"]+))$`)
	typeRegex := regexp.MustCompile(`^(?i)-type=(?:"([^"]+)"|([^\s"]+))$`)
	fsRegex := regexp.MustCompile(`^(?i)-fs=(?:"([^"]+)"|([^\s"]+))$`)

	fmt.Printf("Tokens MKFS recibidos: %v\n", tokens)

	// Iterar sobre cada TOKEN individual
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		fmt.Printf("Procesando token: '%s'\n", token)

		var match []string
		var key string
		var value string
		matched := false // Flag para saber si el token coincidió con alguna regex

		// Intentar hacer match con cada regex válida
		if match = idRegex.FindStringSubmatch(token); match != nil {
			key = "id"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			} // Extraer valor del grupo correcto
			matched = true
		} else if match = typeRegex.FindStringSubmatch(token); match != nil {
			key = "type"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if match = fsRegex.FindStringSubmatch(token); match != nil {
			key = "fs"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		}

		// Si el token NO coincidió con NINGUNA regex válida
		if !matched {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'", token)
		}

		// Procesamiento común para tokens válidos
		fmt.Printf("  Match!: key='%s', value='%s'\n", key, value)

		// Validar duplicados
		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado: -%s", key)
		}
		processedKeys[key] = true

		// Validar valor vacío
		if value == "" {
			return "", fmt.Errorf("el valor para el parámetro -%s no puede estar vacío", key)
		}

		// Asignar y validar valor específico
		switch key {
		case "id":
			cmd.id = value
		case "type":
			typeLower := strings.ToLower(value)
			if typeLower != "full" {
				return "", fmt.Errorf("valor inválido '%s' para -type: debe ser 'full'", value)
			}
			cmd.typ = typeLower
		case "fs":
			fsLower := strings.ToLower(value)
			if fsLower != "2fs" && fsLower != "3fs" {
				return "", fmt.Errorf("valor inválido '%s' para -fs: debe ser '2fs' o '3fs'", value)
			}
			cmd.fs = fsLower
		}
	}

	// Validaciones y Defaults
	if !processedKeys["id"] {
		return "", errors.New("parámetro requerido faltante: -id")
	}
	if !processedKeys["type"] {
		cmd.typ = "full"
		fmt.Println("INFO: Parámetro -type no especificado, usando por defecto 'full'.")
	}
	if !processedKeys["fs"] {
		cmd.fs = "2fs"
		fmt.Println("INFO: Parámetro -fs no especificado, usando por defecto '2fs' (EXT2).")
	}

	err := commandMkfs(cmd)
	if err != nil {
		return "", err
	}

	fsName := "EXT2"
	if cmd.fs == "3fs" {
		fsName = "EXT3"
	}
	return fmt.Sprintf("MKFS: Sistema de archivos %s creado exitosamente\n"+
		"-> ID: %s\n"+
		"-> Tipo: %s",
		fsName, cmd.id, cmd.typ), nil
}

func commandMkfs(mkfs *MKFS) error {
	fmt.Printf("Iniciando formateo MKFS para partición ID: %s, Tipo: %s, Sistema de Archivos: %s\n", mkfs.id, mkfs.typ, mkfs.fs)

	// Obtener Info de la Partición
	_, mountedPartitionInfo, partitionPath, err := stores.GetMountedPartitionInfo(mkfs.id) // Usar la función corregida
	if err != nil {
		return fmt.Errorf("error obteniendo información de la partición '%s': %w", mkfs.id, err)
	}
	fmt.Println("\nInformación de la Partición:")
	mountedPartitionInfo.PrintPartition()
	if mountedPartitionInfo.Part_size <= int32(binary.Size(structures.SuperBlock{}))+1024 {
		return fmt.Errorf("la partición '%s' es demasiado pequeña para formatear", mkfs.id)
	}

	// Calcular n
	n := calculateN(mountedPartitionInfo)
	fmt.Println("\nValor de n calculado:", n)
	minInodes := int32(3)
	if mkfs.fs == "2fs" {
		minInodes = 2
	}
	if n < minInodes {
		return fmt.Errorf("espacio insuficiente para estructuras básicas (n=%d, min=%d)", n, minInodes)
	}

	// Crear SuperBloque Inicial
	superBlock := createSuperBlock(mountedPartitionInfo, n, mkfs.fs)
	if superBlock == nil {
		return errors.New("falló la creación del superbloque inicial")
	}
	fmt.Println("\nSuperBloque Inicial Creado:")
	superBlock.Print()

	// Crear Bitmaps Vacíos
	fmt.Println("Creando bitmaps iniciales...")
	err = superBlock.CreateBitMaps(partitionPath)
	if err != nil {
		return fmt.Errorf("error al crear bitmaps: %w", err)
	}

	// Crear Estructuras Iniciales
	fmt.Println("Creando estructuras iniciales del sistema de archivos...")
	err = createInitialStructures(superBlock, partitionPath, mkfs.fs)
	if err != nil {
		return fmt.Errorf("error al crear estructuras iniciales: %w", err)
	}
	fmt.Println("Estructuras iniciales creadas.")

	// Establecer Punteros a Primer Libre en SuperBloque
	fmt.Println("Buscando primer inodo y bloque libres...")
	nextFreeInodeIdx, errInode := superBlock.FindFreeInode(partitionPath)
	if errInode != nil {
		fmt.Printf("Advertencia: No se encontró inodo libre post-inicialización: %v. S_first_ino=-1.\n", errInode)
		superBlock.S_first_ino = -1
	} else {
		superBlock.S_first_ino = nextFreeInodeIdx
		fmt.Printf("  Primer inodo libre encontrado: %d\n", nextFreeInodeIdx)
	}

	nextFreeBlockIdx, errBlock := superBlock.FindFreeBlock(partitionPath)
	if errBlock != nil {
		fmt.Printf("Advertencia: No se encontró bloque libre post-inicialización: %v. S_first_blo=-1.\n", errBlock)
		superBlock.S_first_blo = -1
	} else {
		superBlock.S_first_blo = nextFreeBlockIdx
		fmt.Printf("  Primer bloque libre encontrado: %d\n", nextFreeBlockIdx)
	}
	fmt.Printf("Superbloque actualizado con S_first_ino=%d, S_first_blo=%d\n", superBlock.S_first_ino, superBlock.S_first_blo)

	// Imprimir y Serializar SuperBloque Final
	fmt.Println("\nSuperBloque Finalizado (antes de serializar):")
	superBlock.Print()
	fmt.Println("Serializando SuperBloque final...")
	err = superBlock.Serialize(partitionPath, int64(mountedPartitionInfo.Part_start))
	if err != nil {
		return fmt.Errorf("error al serializar el superbloque final: %w", err)
	}

	fmt.Println("Formateo MKFS completado.")
	return nil
}

func calculateN(partition *structures.Partition) int32 {
	inodeSize := int32(binary.Size(structures.Inode{}))
	blockSize := int32(binary.Size(structures.FileBlock{}))
	superblockSize := int32(binary.Size(structures.SuperBlock{}))
	if inodeSize <= 0 || blockSize <= 0 || superblockSize <= 0 {
		return 0
	}
	availableSpace := partition.Part_size - superblockSize
	if availableSpace <= 0 {
		return 0
	}
	denominator := int32(1) + int32(3) + inodeSize + (3 * blockSize)
	if denominator <= 0 {
		return 0
	}
	n_float := math.Floor(float64(availableSpace) / float64(denominator))
	if n_float < 0 {
		n_float = 0
	}
	return int32(n_float)
}

func createSuperBlock(partition *structures.Partition, n int32, fsType string) *structures.SuperBlock {
	inodeSize := int32(binary.Size(structures.Inode{}))
	blockSize := int32(binary.Size(structures.FileBlock{}))
	superblockSize := int32(binary.Size(structures.SuperBlock{}))
	if n <= 0 || inodeSize <= 0 || blockSize <= 0 || superblockSize <= 0 {
		return nil
	}
	bm_inode_start := partition.Part_start + superblockSize
	bm_block_start := bm_inode_start + n
	inode_start := bm_block_start + (3 * n)
	block_start := inode_start + (n * inodeSize)
	end_of_blocks := block_start + (3 * n * blockSize)
	partitionEnd := partition.Part_start + partition.Part_size
	if end_of_blocks > partitionEnd {
		return nil
	}
	filesystemTypeVal := int32(2)
	if fsType == "3fs" {
		filesystemTypeVal = 3
	}
	superBlock := &structures.SuperBlock{
		S_filesystem_type: filesystemTypeVal,
		S_inodes_count:    n, S_blocks_count: 3 * n,
		S_free_inodes_count: n, S_free_blocks_count: 3 * n,
		S_mtime: float32(time.Now().Unix()), S_umtime: 0, S_mnt_count: 0,
		S_magic:      0xEF53,
		S_inode_size: inodeSize, S_block_size: blockSize,
		S_first_ino: -1, S_first_blo: -1,
		S_bm_inode_start: bm_inode_start, S_bm_block_start: bm_block_start,
		S_inode_start: inode_start, S_block_start: block_start,
	}
	return superBlock
}

func createInitialStructures(sb *structures.SuperBlock, diskPath string, fsType string) error {
	fmt.Println("Creando estructura de directorio raíz (inodo 0)...")
	inodeRoot := structures.Inode{
		I_uid: 1, I_gid: 1, I_size: 0,
		I_atime: float32(time.Now().Unix()), I_ctime: float32(time.Now().Unix()), I_mtime: float32(time.Now().Unix()),
		I_type: [1]byte{'0'}, I_perm: [3]byte{'7', '7', '5'},
	}
	for i := range inodeRoot.I_block {
		inodeRoot.I_block[i] = -1
	}
	rootBlockIndex, err := sb.FindFreeBlock(diskPath)
	if err != nil {
		return fmt.Errorf("no hay bloques libres para directorio raíz: %w", err)
	}
	inodeRoot.I_block[0] = rootBlockIndex
	if err := sb.UpdateBitmapBlock(diskPath, rootBlockIndex,'1'); err != nil {
		return fmt.Errorf("error bitmap bloque raíz %d: %w", rootBlockIndex, err)
	}
	sb.S_free_blocks_count--
	if err := sb.UpdateBitmapInode(diskPath, 0,'1'); err != nil {
		return fmt.Errorf("error bitmap inodo raíz 0: %w", err)
	}
	sb.S_free_inodes_count--
	rootFolderBlock := structures.FolderBlock{}
	rootFolderBlock.Initialize()
	copy(rootFolderBlock.B_content[0].B_name[:], ".")
	rootFolderBlock.B_content[0].B_inodo = 0
	copy(rootFolderBlock.B_content[1].B_name[:], "..")
	rootFolderBlock.B_content[1].B_inodo = 0
	rootBlockOffset := int64(sb.S_block_start + rootBlockIndex*sb.S_block_size)
	if err := rootFolderBlock.Serialize(diskPath, rootBlockOffset); err != nil {
		return fmt.Errorf("error serializando bloque raíz %d: %w", rootBlockIndex, err)
	}
	rootInodeOffset := int64(sb.S_inode_start)
	if err := inodeRoot.Serialize(diskPath, rootInodeOffset); err != nil {
		return fmt.Errorf("error serializando inodo raíz 0: %w", err)
	}

	fmt.Println("Creando archivo /users.txt (inodo 1)...")
	usersContent := "1,G,root\n1,U,root,root,123\n"
	usersSize := int32(len(usersContent))
	inodeUsers := structures.Inode{
		I_uid: 1, I_gid: 1, I_size: usersSize,
		I_atime: float32(time.Now().Unix()), I_ctime: float32(time.Now().Unix()), I_mtime: float32(time.Now().Unix()),
		I_type: [1]byte{'1'}, I_perm: [3]byte{'6', '6', '4'},
	}
	for i := range inodeUsers.I_block {
		inodeUsers.I_block[i] = -1
	}
	usersBlockIndex, err := sb.FindFreeBlock(diskPath)
	if err != nil {
		return fmt.Errorf("no hay bloques libres para users.txt: %w", err)
	}
	inodeUsers.I_block[0] = usersBlockIndex
	if err := sb.UpdateBitmapBlock(diskPath, usersBlockIndex,'1'); err != nil {
		return fmt.Errorf("error bitmap bloque users %d: %w", usersBlockIndex, err)
	}
	sb.S_free_blocks_count--
	if err := sb.UpdateBitmapInode(diskPath, 1,'1'); err != nil {
		return fmt.Errorf("error bitmap inodo users 1: %w", err)
	}
	sb.S_free_inodes_count--
	usersFileBlock := structures.FileBlock{}
	copy(usersFileBlock.B_content[:], usersContent)
	usersBlockOffset := int64(sb.S_block_start + usersBlockIndex*sb.S_block_size)
	if err := usersFileBlock.Serialize(diskPath, usersBlockOffset); err != nil {
		return fmt.Errorf("error serializando bloque users %d: %w", usersBlockIndex, err)
	}
	usersInodeOffset := int64(sb.S_inode_start + 1*sb.S_inode_size)
	if err := inodeUsers.Serialize(diskPath, usersInodeOffset); err != nil {
		return fmt.Errorf("error serializando inodo users 1: %w", err)
	}

	foundSlotRoot := false
	for i := range rootFolderBlock.B_content {
		if rootFolderBlock.B_content[i].B_inodo == -1 {
			rootFolderBlock.B_content[i].B_inodo = 1
			copy(rootFolderBlock.B_content[i].B_name[:], "users.txt")
			foundSlotRoot = true
			break
		}
	}
	if !foundSlotRoot {
		return errors.New("error interno: no se encontró slot libre en bloque raíz para añadir users.txt")
	}
	if err := rootFolderBlock.Serialize(diskPath, rootBlockOffset); err != nil {
		return fmt.Errorf("error re-serializando bloque raíz %d con entrada users.txt: %w", rootBlockIndex, err)
	}

	if fsType == "3fs" {
		fmt.Println("Creando archivo de journal /.journal (inodo 2)...")
		journalInodeIndex := int32(2)
		journalBlocksNeeded := sb.S_inodes_count / 32
		if journalBlocksNeeded < 16 {
			journalBlocksNeeded = 16
		}
		if journalBlocksNeeded > 1024 {
			journalBlocksNeeded = 1024
		}
		fmt.Printf("  Asignando %d bloques para el journal...\n", journalBlocksNeeded)
		if journalBlocksNeeded > sb.S_free_blocks_count {
			return fmt.Errorf("espacio insuficiente para %d bloques de journal (libres: %d)", journalBlocksNeeded, sb.S_free_blocks_count)
		}
		journalSize := journalBlocksNeeded * sb.S_block_size
		inodeJournal := structures.Inode{
			I_uid: 0, I_gid: 0, I_size: journalSize,
			I_atime: float32(time.Now().Unix()), I_ctime: float32(time.Now().Unix()), I_mtime: float32(time.Now().Unix()),
			I_type: [1]byte{'1'}, I_perm: [3]byte{'6', '0', '0'},
		}
		for i := range inodeJournal.I_block {
			inodeJournal.I_block[i] = -1
		}
		var firstJournalBlockIndex int32 = -1
		for j := int32(0); j < journalBlocksNeeded; j++ {
			if j >= 12 {
				journalSize = 12 * sb.S_block_size
				inodeJournal.I_size = journalSize
				break
			}
			blockIdx, err := sb.FindFreeBlock(diskPath)
			if err != nil {
				return fmt.Errorf("no hay bloques libres para journal (bloque %d): %w", j+1, err)
			}
			if err := sb.UpdateBitmapBlock(diskPath, blockIdx,'1'); err != nil {
				return fmt.Errorf("error bitmap bloque journal %d: %w", blockIdx, err)
			}
			sb.S_free_blocks_count--
			inodeJournal.I_block[j] = blockIdx
			if j == 0 {
				firstJournalBlockIndex = blockIdx
			}
		}
		journalInodeOffset := int64(sb.S_inode_start + journalInodeIndex*sb.S_inode_size)
		if err := inodeJournal.Serialize(diskPath, journalInodeOffset); err != nil {
			return fmt.Errorf("error serializando inodo journal %d: %w", journalInodeIndex, err)
		}
		if err := sb.UpdateBitmapInode(diskPath, journalInodeIndex,'1'); err != nil {
			return fmt.Errorf("error bitmap inodo journal %d: %w", journalInodeIndex, err)
		}
		sb.S_free_inodes_count--
		if firstJournalBlockIndex != -1 {
			firstJournalBlockOffset := int64(sb.S_block_start + firstJournalBlockIndex*sb.S_block_size)
			fmt.Printf("  Inicializando primer bloque de journal (%d) en offset %d...\n", firstJournalBlockIndex, firstJournalBlockOffset)
			initialJournalEntry := structures.Journal{J_count: 0, J_content: structures.Information{I_operation: [10]byte{'C', 'L', 'E', 'A', 'N', 0}, I_date: float32(time.Now().Unix())}}
			journalFile, errOpen := os.OpenFile(diskPath, os.O_WRONLY, 0644)
			if errOpen != nil {
				fmt.Printf("Advertencia: no se pudo abrir disco para escribir entrada inicial de journal: %v\n", errOpen)
			} else {
				_, errSeek := journalFile.Seek(firstJournalBlockOffset, 0)
				if errSeek == nil {
					errWrite := binary.Write(journalFile, binary.LittleEndian, &initialJournalEntry)
					if errWrite != nil {
						fmt.Printf("Advertencia: no se pudo escribir entrada inicial de journal: %v\n", errWrite)
					} else {
						fmt.Println("  Entrada inicial de journal escrita.")
					}
				} else {
					fmt.Printf("Advertencia: no se pudo buscar offset para entrada inicial de journal: %v\n", errSeek)
				}
				journalFile.Close()
			}
		}
		foundSlotRootJournal := false
		for i := range rootFolderBlock.B_content {
			if rootFolderBlock.B_content[i].B_inodo == -1 {
				rootFolderBlock.B_content[i].B_inodo = journalInodeIndex
				copy(rootFolderBlock.B_content[i].B_name[:], ".journal")
				foundSlotRootJournal = true
				break
			}
		}
		if !foundSlotRootJournal {
			fmt.Println("Advertencia: No se encontró slot libre en bloque raíz para añadir '.journal'.")
		} else {
			fmt.Println("Añadiendo entrada '.journal' al bloque raíz.")
			if err := rootFolderBlock.Serialize(diskPath, rootBlockOffset); err != nil {
				return fmt.Errorf("error re-serializando bloque raíz %d con entrada .journal: %w", rootBlockIndex, err)
			}
		}
	}
	fmt.Println("Estructuras iniciales creadas exitosamente.")
	return nil
}
