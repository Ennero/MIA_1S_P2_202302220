package commands

import (
	"bytes"          
	"encoding/binary" 
	"errors"
	"fmt"
	"os" 
	"regexp"
	"strings"
	"time"

	stores "backend/stores"
	structures "backend/structures"
)

type RECOVERY struct {
	ID string
}

func ParseRecovery(tokens []string) (string, error) {
	cmd := &RECOVERY{}
	processedKeys := make(map[string]bool)

	// Regex para -id=<valor>
	idRegex := regexp.MustCompile(`^(?i)-id=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -id=<valor>")
	}

	// Procesar tokens (solo debe haber uno para -id)
	if len(tokens) > 1 {
		return "", fmt.Errorf("demasiados parámetros para recovery, solo se espera -id=<valor>")
	}
	token := strings.TrimSpace(tokens[0])
	if token == "" {
		return "", errors.New("parámetro vacío proporcionado")
	}

	match := idRegex.FindStringSubmatch(token)
	if match != nil {
		key := "-id"
		value := ""
		if match[1] != "" {
			value = match[1]
		} else {
			value = match[2]
		}

		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado: %s", key)
		}
		processedKeys[key] = true
		if value == "" {
			return "", errors.New("el valor para -id no puede estar vacío")
		}
		cmd.ID = value
	} else {
		return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -id=<valor>", token)
	}

	if !processedKeys["-id"] {
		return "", errors.New("falta el parámetro requerido: -id")
	}

	// Llamar a la lógica del comando
	err := commandRecovery(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("RECOVERY: Recuperación simulada en partición '%s' completada.", cmd.ID), nil
}

func commandRecovery(cmd *RECOVERY) error {
	fmt.Printf("Iniciando recuperación para partición ID: %s\n", cmd.ID)

	// 1. Obtener SB, Partición, Path
	sb, partition, diskPath, err := stores.GetMountedPartitionSuperblock(cmd.ID)
	if err != nil {
		return fmt.Errorf("error obteniendo partición montada '%s': %w", cmd.ID, err)
	}
	if sb.S_magic != 0xEF53 {
		return errors.New("magia de superbloque inválida")
	}
	if sb.S_inode_size <= 0 || sb.S_block_size <= 0 {
		return errors.New("tamaño de inodo o bloque inválido")
	}

	// VERIFICAR QUE SEA EXT3
	if sb.S_filesystem_type != 3 {
		return fmt.Errorf("error: el comando recovery solo aplica a sistemas de archivos EXT3 (tipo detectado: %d)", sb.S_filesystem_type)
	}
	fmt.Println("Sistema de archivos EXT3 confirmado.")

	// Encontrar y Leer Inodo del Journal (Asumiendo Inodo 2)
	fmt.Println("Buscando inodo del journal (/.journal, inodo 2)...")
	journalInodeIndex := int32(2)
	journalInode := &structures.Inode{}
	journalInodeOffset := int64(sb.S_inode_start + journalInodeIndex*sb.S_inode_size)
	if err := journalInode.Deserialize(diskPath, journalInodeOffset); err != nil {
		// Si no se puede leer el inodo del journal, la recuperación no es posible
		return fmt.Errorf("error crítico: no se pudo leer el inodo del journal (%d): %w", journalInodeIndex, err)
	}
	if journalInode.I_type[0] != '1' {
		return fmt.Errorf("error crítico: el inodo del journal (%d) no es de tipo archivo", journalInodeIndex)
	}
	fmt.Printf("Inodo del journal encontrado (Tamaño: %d bytes)\n", journalInode.I_size)

	// Leer Contenido Completo del Journal
	fmt.Println("Leyendo contenido del archivo journal...")
	journalContentStr, errRead := structures.ReadFileContent(sb, diskPath, journalInode)
	if errRead != nil {
		// Si no podemos leer el journal, no podemos recuperar
		return fmt.Errorf("error leyendo contenido del archivo journal: %w", errRead)
	}
	journalContentBytes := []byte(journalContentStr)
	fmt.Printf("Contenido del journal leído: %d bytes\n", len(journalContentBytes))

	// Deserializar Entradas del Journal
	journalEntries := []structures.Journal{}
	journalEntrySize := int(binary.Size(structures.Journal{}))
	if journalEntrySize <= 0 {
		return errors.New("tamaño de struct Journal inválido")
	}

	reader := bytes.NewReader(journalContentBytes)
	entryCount := 0
	for reader.Len() >= journalEntrySize {
		var entry structures.Journal
		err := binary.Read(reader, binary.LittleEndian, &entry)
		if err != nil {
			fmt.Printf("Advertencia: Error deserializando entrada de journal #%d: %v. Deteniendo lectura.\n", entryCount+1, err)
			break 
		}
		// Podríamos validar J_count si tuviera sentido aquí, pero en la estructura actual no mucho
		journalEntries = append(journalEntries, entry)
		entryCount++
	}
	fmt.Printf("Deserializadas %d entradas del journal.\n", len(journalEntries))

	// Analizar Journal y Marcar Recursos Requeridos (Simplificado)
	requiredInodes := make(map[int32]bool)
	requiredBlocks := make(map[int32]bool)

	// Siempre marcar inodos/bloques iniciales como requeridos
	requiredInodes[0] = true // Raíz
	requiredInodes[1] = true // users.txt
	requiredInodes[2] = true // .journal (porque estamos en ext3)
	// Bloques iniciales
	// Releer inodos 0, 1, 2 para obtener sus bloques
	for _, idx := range []int32{0, 1, 2} {
		tempInode := &structures.Inode{}
		tempOffset := int64(sb.S_inode_start + idx*sb.S_inode_size)
		if tempInode.Deserialize(diskPath, tempOffset) == nil {
			for _, blockPtr := range tempInode.I_block {
				if blockPtr != -1 {
					requiredBlocks[blockPtr] = true
				}
			}
		}
	}

	// Iterar sobre las operaciones logueadas
	fmt.Println("Analizando operaciones del journal para marcar recursos...")
	for _, entry := range journalEntries {
		op := strings.TrimRight(string(entry.J_content.I_operation[:]), "\x00 ")
		path := strings.TrimRight(string(entry.J_content.I_path[:]), "\x00 ")
		fmt.Printf("  Procesando log: Op='%s', Path='%s'\n", op, path)

		inodeIdx, inode, errFind := structures.FindInodeByPath(sb, diskPath, path) // Necesita que el path exista!
		if errFind == nil {
			fmt.Printf("    Operación en Inodo %d. Marcando como requerido.\n", inodeIdx)
			requiredInodes[inodeIdx] = true
			for _, blockPtr := range inode.I_block {
				if blockPtr != -1 {
					fmt.Printf("      Marcando bloque %d como requerido.\n", blockPtr)
					requiredBlocks[blockPtr] = true
				}
			}

		} else {
			fmt.Printf("    Advertencia: No se pudo encontrar inodo para path '%s' del journal: %v. No se pueden marcar sus recursos.\n", path, errFind)
			return fmt.Errorf("error crítico: no se pudo encontrar inodo para path '%s' del journal: %w", path, errFind)
		}
	}

	// Reconciliar Bitmaps
	fmt.Println("Reconciliando bitmaps con información del journal...")
	file, errOpen := os.OpenFile(diskPath, os.O_RDWR, 0644)
	if errOpen != nil {
		return fmt.Errorf("error abriendo disco para actualizar bitmaps: %w", errOpen)
	}
	defer file.Close()

	// Bitmap de Inodos
	inodeBitmapChanged := false
	inodeBitmap := make([]byte, sb.S_inodes_count)
	if _, err := file.ReadAt(inodeBitmap, int64(sb.S_bm_inode_start)); err != nil {
		return fmt.Errorf("error leyendo bitmap inodos para recovery: %w", err)
	}
	for inodeIdx := range requiredInodes {
		if inodeIdx >= 0 && inodeIdx < sb.S_inodes_count {
			if inodeBitmap[inodeIdx] == '0' {
				fmt.Printf("  Corrigiendo bitmap inodo: índice %d marcado como '1'\n", inodeIdx)
				inodeBitmap[inodeIdx] = '1' // Marcar como usado
				inodeBitmapChanged = true
			}
		}
	}
	// Reescribir bitmap de inodos si cambió
	if inodeBitmapChanged {
		fmt.Println("  Escribiendo bitmap de inodos actualizado...")
		if _, err := file.WriteAt(inodeBitmap, int64(sb.S_bm_inode_start)); err != nil {
			return fmt.Errorf("error escribiendo bitmap inodos actualizado: %w", err)
		}
	} else {
		fmt.Println("  Bitmap de inodos ya consistente.")
	}

	// Bitmap de Bloques
	blockBitmapChanged := false
	blockBitmap := make([]byte, sb.S_blocks_count)
	if _, err := file.ReadAt(blockBitmap, int64(sb.S_bm_block_start)); err != nil {
		return fmt.Errorf("error leyendo bitmap bloques para recovery: %w", err)
	}
	for blockIdx := range requiredBlocks {
		if blockIdx >= 0 && blockIdx < sb.S_blocks_count {
			if blockBitmap[blockIdx] == '0' {
				fmt.Printf("  Corrigiendo bitmap bloque: índice %d marcado como '1'\n", blockIdx)
				blockBitmap[blockIdx] = '1' // Marcar como usado
				blockBitmapChanged = true
			}
		}
	}
	// Reescribir bitmap de bloques si cambió
	if blockBitmapChanged {
		fmt.Println("  Escribiendo bitmap de bloques actualizado...")
		if _, err := file.WriteAt(blockBitmap, int64(sb.S_bm_block_start)); err != nil {
			return fmt.Errorf("error escribiendo bitmap bloques actualizado: %w", err)
		}
	} else {
		fmt.Println("  Bitmap de bloques ya consistente.")
	}

	// Recalcular Contadores Libres
	fmt.Println("Recalculando contadores libres...")
	freeInodes := int32(0)
	for _, state := range inodeBitmap {
		if state == '0' {
			freeInodes++
		}
	}
	freeBlocks := int32(0)
	for _, state := range blockBitmap {
		if state == '0' {
			freeBlocks++
		}
	}

	fmt.Printf("  Recuento final: Inodos Libres=%d, Bloques Libres=%d\n", freeInodes, freeBlocks)
	sb.S_free_inodes_count = freeInodes
	sb.S_free_blocks_count = freeBlocks

	// Actualizar Tiempo y Serializar Superbloque
	sb.S_mtime = float32(time.Now().Unix()) 
	fmt.Println("Serializando SuperBloque recuperado...")
	err = sb.Serialize(diskPath, int64(partition.Part_start))
	if err != nil {
		return fmt.Errorf("error al serializar superbloque recuperado: %w", err)
	}

	fmt.Println("RECOVERY completado.")
	return nil
}