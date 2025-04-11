package commands

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	stores "backend/stores"
)

type LOSS struct {
	ID string 
}

func ParseLoss(tokens []string) (string, error) {
	cmd := &LOSS{}
	processedKeys := make(map[string]bool)

	// Regex para -id=<valor>
	idRegex := regexp.MustCompile(`^(?i)-id=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -id=<valor>")
	}

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" { continue }

		match := idRegex.FindStringSubmatch(token)
		if match != nil { // Coincide con -id=...
			key := "-id"
			value := ""
			if match[1] != "" { value = match[1] } else { value = match[2] } // Extraer valor

			if processedKeys[key] { return "", fmt.Errorf("parámetro duplicado: %s", key) }
			processedKeys[key] = true
			if value == "" { return "", errors.New("el valor para -id no puede estar vacío") }

			cmd.ID = value // Asignar ID

		} else {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -id=<valor>", token)
		}
	}

	if !processedKeys["-id"] { return "", errors.New("falta el parámetro requerido: -id") }

	err := commandLoss(cmd)
	if err != nil { return "", err }

	return fmt.Sprintf("LOSS: Simulación de pérdida en partición '%s' completada.", cmd.ID), nil
}

func commandLoss(cmd *LOSS) error {
	fmt.Printf("Iniciando simulación de pérdida para partición ID: %s\n", cmd.ID)

	// Obtener Superbloque, Partición y Path del Disco
	sb, partition, diskPath, err := stores.GetMountedPartitionSuperblock(cmd.ID)
	if err != nil { return fmt.Errorf("error obteniendo partición montada '%s': %w", cmd.ID, err) }

	// Validar datos del Superbloque
	if sb.S_magic != 0xEF53 { return fmt.Errorf("magia inválida (0x%X) en superbloque de partición '%s'", sb.S_magic, cmd.ID) }
	if sb.S_inode_size <= 0 || sb.S_block_size <= 0 || sb.S_inodes_count <= 0 || sb.S_blocks_count <= 0 {
		return fmt.Errorf("metadatos de tamaño/conteo inválidos en superbloque de partición '%s'", cmd.ID)
	}
	fmt.Println("Superbloque leído:")
	sb.Print() // Mostrar info antes de borrar

	// 2. Calcular Offsets y Tamaños de las áreas a borrar
	// Es crucial usar los valores del Superbloque (sb)
	bmInodeOffset := int64(sb.S_bm_inode_start)
	bmInodeSize := int64(sb.S_inodes_count) // 1 byte por inodo en bitmap

	bmBlockOffset := int64(sb.S_bm_block_start)
	bmBlockSize := int64(sb.S_blocks_count) // 1 byte por bloque en bitmap

	inodeTableOffset := int64(sb.S_inode_start)
	inodeTableSize := int64(sb.S_inodes_count) * int64(sb.S_inode_size)

	blocksAreaOffset := int64(sb.S_block_start)
	blocksAreaSize := int64(sb.S_blocks_count) * int64(sb.S_block_size)

	// Validación extra: Asegurar que las áreas estén dentro de la partición física
	partitionStart := int64(partition.Part_start)
	partitionEnd := partitionStart + int64(partition.Part_size)

	if bmInodeOffset < partitionStart || (bmInodeOffset+bmInodeSize) > partitionEnd ||
		bmBlockOffset < partitionStart || (bmBlockOffset+bmBlockSize) > partitionEnd ||
		inodeTableOffset < partitionStart || (inodeTableOffset+inodeTableSize) > partitionEnd ||
		blocksAreaOffset < partitionStart || (blocksAreaOffset+blocksAreaSize) > partitionEnd {
		fmt.Printf("Particion Start: %d, End: %d\n", partitionStart, partitionEnd)
		fmt.Printf("BM Inode : %d -> %d (Size: %d)\n", bmInodeOffset, bmInodeOffset+bmInodeSize, bmInodeSize)
		fmt.Printf("BM Block : %d -> %d (Size: %d)\n", bmBlockOffset, bmBlockOffset+bmBlockSize, bmBlockSize)
		fmt.Printf("Inode Tbl: %d -> %d (Size: %d)\n", inodeTableOffset, inodeTableOffset+inodeTableSize, inodeTableSize)
		fmt.Printf("Blocks   : %d -> %d (Size: %d)\n", blocksAreaOffset, blocksAreaOffset+blocksAreaSize, blocksAreaSize)
		return errors.New("error de consistencia: las áreas del sistema de archivos calculadas exceden los límites de la partición física")
	}

	// Abrir archivo en modo Escritura
	fmt.Printf("Abriendo disco '%s' para escritura...\n", diskPath)
	file, errOpen := os.OpenFile(diskPath, os.O_RDWR, 0644) // Necesitamos RDWR
	if errOpen != nil { return fmt.Errorf("error al abrir disco '%s' para escritura: %w", diskPath, errOpen) }
	defer file.Close() // Asegurar cierre

	// Sobrescribir cada área con ceros
	fmt.Printf("-> Borrando Bitmap de Inodos (Offset: %d, Size: %d)...\n", bmInodeOffset, bmInodeSize)
	if err := zeroOutSpace(file, bmInodeOffset, bmInodeSize); err != nil {
		return fmt.Errorf("error borrando bitmap de inodos: %w", err)
	}

	fmt.Printf("-> Borrando Bitmap de Bloques (Offset: %d, Size: %d)...\n", bmBlockOffset, bmBlockSize)
	if err := zeroOutSpace(file, bmBlockOffset, bmBlockSize); err != nil {
		return fmt.Errorf("error borrando bitmap de bloques: %w", err)
	}

	fmt.Printf("-> Borrando Tabla de Inodos (Offset: %d, Size: %d)...\n", inodeTableOffset, inodeTableSize)
	if err := zeroOutSpace(file, inodeTableOffset, inodeTableSize); err != nil {
		return fmt.Errorf("error borrando tabla de inodos: %w", err)
	}

	fmt.Printf("-> Borrando Área de Bloques (Offset: %d, Size: %d)...\n", blocksAreaOffset, blocksAreaSize)
	if err := zeroOutSpace(file, blocksAreaOffset, blocksAreaSize); err != nil {
		return fmt.Errorf("error borrando área de bloques: %w", err)
	}

	fmt.Println("Simulación de pérdida completada.")
	return nil
}