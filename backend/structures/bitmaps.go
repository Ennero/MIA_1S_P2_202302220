package structures

import (
	"fmt"
	"os"
)

// inicializando como libres 
func (sb *SuperBlock) CreateBitMaps(path string) error {
	// Abrir archivo para escritura, creándolo si no existe
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		// Mejoramos el mensaje de error
		return fmt.Errorf("error al abrir/crear archivo para bitmaps (%s): %w", path, err)
	}
	defer file.Close()

	// --- Bitmap de inodos ---
	// Validar que el conteo de inodos sea positivo
	if sb.S_inodes_count <= 0 {
		return fmt.Errorf("el número total de inodos (S_inodes_count) es inválido: %d", sb.S_inodes_count)
	}
	// Mover el puntero del archivo a la posición inicial del bitmap de inodos
	_, err = file.Seek(int64(sb.S_bm_inode_start), 0)
	if err != nil {
		return fmt.Errorf("error al buscar inicio de bitmap de inodos (offset %d): %w", sb.S_bm_inode_start, err)
	}

	//libre
	inodeBitmapBuffer := make([]byte, sb.S_inodes_count)
	for i := range inodeBitmapBuffer {
		inodeBitmapBuffer[i] = '0' // '0' representa un inodo libre
	}

	// Escribir el buffer del bitmap de inodos en el archivo
	bytesWritten, err := file.Write(inodeBitmapBuffer)
	if err != nil {
		return fmt.Errorf("error al escribir bitmap de inodos: %w", err)
	}
	if bytesWritten != len(inodeBitmapBuffer) {
		return fmt.Errorf("escritura incompleta del bitmap de inodos (escritos %d, esperados %d)", bytesWritten, len(inodeBitmapBuffer))
	}

	// Validar que el conteo de bloques sea positivo
	if sb.S_blocks_count <= 0 {
		return fmt.Errorf("el número total de bloques (S_blocks_count) es inválido: %d", sb.S_blocks_count)
	}
	// Mover el puntero del archivo a la posición inicial del bitmap de bloques
	_, err = file.Seek(int64(sb.S_bm_block_start), 0)
	if err != nil {
		return fmt.Errorf("error al buscar inicio de bitmap de bloques (offset %d): %w", sb.S_bm_block_start, err)
	}

	//libre
	blockBitmapBuffer := make([]byte, sb.S_blocks_count)
	for i := range blockBitmapBuffer {
		blockBitmapBuffer[i] = '0'
	}

	// Escribir el buffer del bitmap de bloques en el archivo
	bytesWritten, err = file.Write(blockBitmapBuffer)
	if err != nil {
		return fmt.Errorf("error al escribir bitmap de bloques: %w", err)
	}
	if bytesWritten != len(blockBitmapBuffer) {
		return fmt.Errorf("escritura incompleta del bitmap de bloques (escritos %d, esperados %d)", bytesWritten, len(blockBitmapBuffer))
	}

	fmt.Println("Bitmaps creados e inicializados correctamente.")
	return nil
}

// Marca el inodo en el índice especificado como ocupado 
func (sb *SuperBlock) UpdateBitmapInode(path string, inodeIndex int32) error {
	// Validación del índice proporcionado
	if inodeIndex < 0 || inodeIndex >= sb.S_inodes_count {
		return fmt.Errorf("índice de inodo fuera de rango: %d (total de inodos: %d)", inodeIndex, sb.S_inodes_count)
	}

	// Abrir el archivo para lectura Y escritura
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error al abrir archivo para actualizar bitmap de inodos (%s): %w", path, err)
	}
	defer file.Close()

	// Calcular el offset EXACTO dentro del bitmap para el índice dado.
	offset := int64(sb.S_bm_inode_start) + int64(inodeIndex)
	_, err = file.Seek(offset, 0)
	if err != nil {
		return fmt.Errorf("error al buscar posición %d en bitmap de inodos: %w", offset, err)
	}

	// Escribir el byte ocupado en la posición calculada.
	bytesWritten, err := file.Write([]byte{'1'})
	if err != nil {
		return fmt.Errorf("error al escribir '1' en bitmap de inodos en índice %d (offset %d): %w", inodeIndex, offset, err)
	}
	if bytesWritten != 1 {
		return fmt.Errorf("error al actualizar bitmap de inodos: se esperaban escribir 1 byte pero se escribieron %d", bytesWritten)
	}

	fmt.Printf("Bitmap de inodo actualizado en índice: %d\n", inodeIndex)
	return nil
}

// UpdateBitmapBlock marca el bloque en el índice especificado como ocupado
func (sb *SuperBlock) UpdateBitmapBlock(path string, blockIndex int32) error {
	// Validación del índice proporcionado
	if blockIndex < 0 || blockIndex >= sb.S_blocks_count {
		return fmt.Errorf("índice de bloque fuera de rango: %d (total de bloques: %d)", blockIndex, sb.S_blocks_count)
	}

	// Abrir el archivo para lectura Y escritura
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error al abrir archivo para actualizar bitmap de bloques (%s): %w", path, err)
	}
	defer file.Close()

	// Calcular el offset EXACTO dentro del bitmap para el índice dado.
	offset := int64(sb.S_bm_block_start) + int64(blockIndex)
	_, err = file.Seek(offset, 0)
	if err != nil {
		return fmt.Errorf("error al buscar posición %d en bitmap de bloques: %w", offset, err)
	}

	// Escribir el byte '1' (ocupado) en la posición calculada.
	bytesWritten, err := file.Write([]byte{'1'})
	if err != nil {
		return fmt.Errorf("error al escribir '1' en bitmap de bloques en índice %d (offset %d): %w", blockIndex, offset, err)
	}
	// Verificar que se escribió exactamente 1 byte
	if bytesWritten != 1 {
		return fmt.Errorf("error al actualizar bitmap de bloques: se esperaban escribir 1 byte pero se escribieron %d", bytesWritten)
	}

	// fmt.Printf("Bitmap de bloque actualizado en índice: %d\n", blockIndex) // Mensaje opcional
	return nil
}