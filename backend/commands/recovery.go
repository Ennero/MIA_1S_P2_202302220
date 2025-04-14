package commands

import (
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
	fmt.Printf("Iniciando recuperación SIMPLE para partición ID: %s\n", cmd.ID)

	// Obtener SB, Partición, Path
	sb, partition, diskPath, err := stores.GetMountedPartitionSuperblock(cmd.ID)
	if err != nil {
		return fmt.Errorf("error obteniendo partición '%s': %w", cmd.ID, err)
	}
	if sb.S_magic != 0xEF53 {
		return fmt.Errorf("magia SB inválida en '%s'", cmd.ID)
	}
	if sb.S_inode_size <= 0 || sb.S_block_size <= 0 || sb.S_inodes_count <= 0 || sb.S_blocks_count <= 0 {
		return fmt.Errorf("metadatos inválidos en SB '%s'", cmd.ID)
	}

	// VERIFICAR QUE SEA EXT3
	if sb.S_filesystem_type != 3 {
		return fmt.Errorf("error: recovery solo aplica a EXT3 (tipo detectado: %d)", sb.S_filesystem_type)
	}
	fmt.Println("Sistema de archivos EXT3 confirmado.")

	// Leer journal solo para ver si existe y es legible mínimamente
	journalInodeIndex := int32(2)
	journalInode := &structures.Inode{}
	journalInodeOffset := int64(sb.S_inode_start + journalInodeIndex*sb.S_inode_size)
	if err := journalInode.Deserialize(diskPath, journalInodeOffset); err != nil {
		fmt.Printf("Advertencia: No se pudo leer inodo journal %d: %v\n", journalInodeIndex, err)
		return fmt.Errorf("error crítico: no se pudo leer el inodo del journal (%d): %w", journalInodeIndex, err)
	} else if journalInode.I_type[0] != '1' {
		fmt.Printf("Advertencia: Inodo journal %d no es tipo archivo.\n", journalInodeIndex)
	} else {
		fmt.Printf("Inodo del journal %d encontrado y es tipo archivo.\n", journalInodeIndex)
	}

	// Marcar Recursos INICIALES Requeridos
	requiredInodes := make(map[int32]bool)
	requiredBlocks := make(map[int32]bool)

	fmt.Println("Marcando inodos/bloques iniciales (0, 1, 2) como requeridos...")
	initialInodes := []int32{0, 1, 2} // Inodos para /, users.txt, .journal

	for _, idx := range initialInodes {
		if idx >= sb.S_inodes_count { // Chequeo por si n < 3
			fmt.Printf("Advertencia: Inodo inicial %d excede S_inodes_count (%d).\n", idx, sb.S_inodes_count)
			continue
		}
		requiredInodes[idx] = true // Marcar inodo como necesario

		// Implementación con Asunción de Bloques Contiguos Iniciales ---
		fmt.Printf("  Asumiendo bloques iniciales para Inodo %d...\n", idx)
		switch idx {
		case 0: // Raíz '/'
			if 0 < sb.S_blocks_count {
				requiredBlocks[0] = true
				fmt.Println("    -> Bloque 0 requerido (para Inodo 0)")
			}
		case 1: // users.txt
			if 1 < sb.S_blocks_count {
				requiredBlocks[1] = true
				fmt.Println("    -> Bloque 1 requerido (para Inodo 1)")
			}
		case 2: 
			// Calcular cuántos bloques directos USÓ el journal
			journalBlocksNeeded := sb.S_inodes_count / 32 
			if journalBlocksNeeded < 16 {
				journalBlocksNeeded = 16
			}
			if journalBlocksNeeded > 1024 {
				journalBlocksNeeded = 1024
			}
			// Limitar a punteros directos
			if journalBlocksNeeded > 12 {
				journalBlocksNeeded = 12
			}
			fmt.Printf("    Asumiendo %d bloques para Journal (Inodo 2)...\n", journalBlocksNeeded)
			startBlockJournal := int32(2) // Asumiendo que empieza en bloque 2
			for k := int32(0); k < journalBlocksNeeded; k++ {
				blockIdx := startBlockJournal + k
				if blockIdx < sb.S_blocks_count {
					requiredBlocks[blockIdx] = true
					fmt.Printf("      -> Bloque %d requerido (para Inodo 2)\n", blockIdx)
				} else {
					fmt.Printf("      Advertencia: Bloque asumido %d para journal excede S_blocks_count (%d).\n", blockIdx, sb.S_blocks_count)
					break
				}
			}
		}
	}

	// Reconciliar Bitmaps (Marcar como '1' los requeridos)
	fmt.Println("Reconciliando bitmaps con información inicial...")
	file, errOpen := os.OpenFile(diskPath, os.O_RDWR, 0644)
	if errOpen != nil {
		return fmt.Errorf("error abriendo disco para actualizar bitmaps: %w", errOpen)
	}
	defer file.Close()

	// Bitmap de Inodos
	inodeBitmapChanged := false
	inodeBitmap := make([]byte, sb.S_inodes_count) // Leer el bitmap
	if _, err := file.ReadAt(inodeBitmap, int64(sb.S_bm_inode_start)); err != nil {
		return fmt.Errorf("error leyendo bitmap inodos: %w", err)
	}
	for inodeIdx := range requiredInodes {
		if inodeIdx >= 0 && inodeIdx < sb.S_inodes_count {
			if inodeBitmap[inodeIdx] != '1' { // Si no está marcado como usado
				fmt.Printf("  Corrigiendo bitmap inodo: índice %d marcado como '1'\n", inodeIdx)
				inodeBitmap[inodeIdx] = '1'
				inodeBitmapChanged = true
			}
		}
	}
	if inodeBitmapChanged { // Reescribir solo si hubo cambios
		fmt.Println("  Escribiendo bitmap de inodos actualizado...")
		if _, err := file.WriteAt(inodeBitmap, int64(sb.S_bm_inode_start)); err != nil {
			return fmt.Errorf("error escribiendo bitmap inodos: %w", err)
		}
	} else {
		fmt.Println("  Bitmap de inodos ya consistente con estado inicial.")
	}

	// Bitmap de Bloques
	blockBitmapChanged := false
	blockBitmap := make([]byte, sb.S_blocks_count) // Leer bitmap bloques
	if _, err := file.ReadAt(blockBitmap, int64(sb.S_bm_block_start)); err != nil {
		return fmt.Errorf("error leyendo bitmap bloques: %w", err)
	}
	for blockIdx := range requiredBlocks {
		if blockIdx >= 0 && blockIdx < sb.S_blocks_count {
			if blockBitmap[blockIdx] != '1' { // Si no está marcado como usado
				fmt.Printf("  Corrigiendo bitmap bloque: índice %d marcado como '1'\n", blockIdx)
				blockBitmap[blockIdx] = '1'
				blockBitmapChanged = true
			}
		}
	}
	if blockBitmapChanged { // Reescribir solo si hubo cambios
		fmt.Println("  Escribiendo bitmap de bloques actualizado...")
		if _, err := file.WriteAt(blockBitmap, int64(sb.S_bm_block_start)); err != nil {
			return fmt.Errorf("error escribiendo bitmap bloques: %w", err)
		}
	} else {
		fmt.Println("  Bitmap de bloques ya consistente con estado inicial.")
	}

	// Recalcular Contadores Libres en SB
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

	sb.S_first_ino = 3                       // Asumiendo que 0, 1, 2 están usados
	lastJournalBlock := int32(2) + int32(12) // Estimación máxima de bloque usado por journal directo
	if sb.S_inodes_count/32 > 12 {
		lastJournalBlock = 2 + 12 - 1
	} else {
		lastJournalBlock = 2 + sb.S_inodes_count/32 - 1
	} 
	if lastJournalBlock < 2 {
		lastJournalBlock = 2
	} 
	sb.S_first_blo = lastJournalBlock + 1 // El siguiente al último bloque del journal (asumiendo contiguo)

	// Actualizar Tiempo y Serializar Superbloque
	sb.S_mtime = float32(time.Now().Unix()) // Hora de la recuperación
	fmt.Println("Serializando SuperBloque recuperado...")
	err = sb.Serialize(diskPath, int64(partition.Part_start))
	if err != nil {
		return fmt.Errorf("error al serializar superbloque recuperado: %w", err)
	}

	fmt.Println("RECOVERY (simple) completado.")
	return nil
}
