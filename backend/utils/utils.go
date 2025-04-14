package utils

import (
	"backend/structures"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ConvertToBytes(size int, unit string) (int, error) {
	// Convertir la unidad a mayúsculas para hacer la comparación case-insensitive
	unitUpper := strings.ToUpper(unit)

	switch unitUpper {
	case "B":
		return size, nil
	case "K":
		return size * 1024, nil
	case "M":
		return size * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("unidad inválida: '%s'. Solo se aceptan B, K o M", unit)
	}
}

// Lista con todo el abecedario
var alphabet = []string{
	"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
	"N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
}

// Mapa para almacenar la asignación de letras a los diferentes paths
var pathToLetter = make(map[string]string)

// Mapa para almacenar el contador de particiones por path
var pathToPartitionCount = make(map[string]int)

// Índice para la siguiente letra disponible en el abecedario
var nextLetterIndex = 0

// GetLetter obtiene la letra asignada a un path y el siguiente índice de partición
func GetLetterAndPartitionCorrelative(path string) (string, int, error) {
	// Asignar una letra al path si no tiene una asignada
	if _, exists := pathToLetter[path]; !exists {
		if nextLetterIndex < len(alphabet) {
			pathToLetter[path] = alphabet[nextLetterIndex]
			pathToPartitionCount[path] = 0 // Inicializar el contador de particiones
			nextLetterIndex++
		} else {
			fmt.Println("Error: no hay más letras disponibles para asignar")
			return "", 0, errors.New("no hay más letras disponibles para asignar")
		}
	}

	// Incrementar y obtener el siguiente índice de partición para este path
	pathToPartitionCount[path]++
	nextIndex := pathToPartitionCount[path]

	return pathToLetter[path], nextIndex, nil
}

// createParentDirs crea las carpetas padre si no existen
func CreateParentDirs(path string) error {
	dir := filepath.Dir(path)
	// os.MkdirAll no sobrescribe las carpetas existentes, solo crea las que no existen
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("error al crear las carpetas padre: %v", err)
	}
	return nil
}

func GetFileNames(path string) (string, string) {
	// Limpiar el path
	cleanedPath := filepath.Clean(path)

	//Extraer componentes del path
	dir := filepath.Dir(cleanedPath)
	ext := filepath.Ext(cleanedPath)
	baseName := strings.TrimSuffix(filepath.Base(cleanedPath), ext)

	// Construir el nombre del archivo .dot
	dotFileName := filepath.Join(dir, baseName+".dot")
	outputImage := filepath.Join(dir, baseName+ext) // /ruta/a/reporte.png

	return dotFileName, outputImage
}

// GetParentDirectories obtiene las carpetas padres y el directorio de destino
func GetParentDirectories(path string) ([]string, string) {
	// Normalizar el path
	path = filepath.Clean(path)

	// Dividir el path en sus componentes
	components := strings.Split(path, string(filepath.Separator))

	// Lista para almacenar las rutas de las carpetas padres
	var parentDirs []string

	// Construir las rutas de las carpetas padres, excluyendo la última carpeta
	for i := 1; i < len(components)-1; i++ {
		parentDirs = append(parentDirs, components[i])
	}

	// La última carpeta es la carpeta de destino
	destDir := components[len(components)-1]

	return parentDirs, destDir
}

// First devuelve el primer elemento de un slice
func First[T any](slice []T) (T, error) {
	if len(slice) == 0 {
		var zero T
		return zero, errors.New("el slice está vacío")
	}
	return slice[0], nil
}

// RemoveElement elimina un elemento de un slice en el índice dado
func RemoveElement[T any](slice []T, index int) []T {
	if index < 0 || index >= len(slice) {
		return slice // Índice fuera de rango, devolver el slice original
	}
	return append(slice[:index], slice[index+1:]...)
}

// splitStringIntoChunks divide una cadena en partes de tamaño chunkSize y las almacena en una lista
func SplitStringIntoChunks(s string) []string {
	var chunks []string
	for i := 0; i < len(s); i += 64 {
		end := i + 64
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}

// Función para obtener el nombre del disco
func GetDiskName(path string) string {
	return filepath.Base(path)
}

//--------------------------------------------------------------------------------------------------------------------------------------------------
// PARA EL JOURNALING

func AppendToJournal(entryData structures.Information, sb *structures.SuperBlock, diskPath string) error {
	fmt.Println("--> appendToJournal: Añadiendo entrada al journal...")

	// Obtener inodo del journal (siempre inodo 2)
	journalInodeIndex := int32(2)
	journalInode := &structures.Inode{}
	journalInodeOffset := int64(sb.S_inode_start + journalInodeIndex*sb.S_inode_size)
	if err := journalInode.Deserialize(diskPath, journalInodeOffset); err != nil {
		return fmt.Errorf("appendToJournal: error crítico leyendo inodo journal %d: %w", journalInodeIndex, err)
	}
	if journalInode.I_type[0] != '1' {
		return fmt.Errorf("appendToJournal: inodo journal %d no es archivo", journalInodeIndex)
	}

	// Calcular offset de escritura (final actual del archivo journal)
	writeOffsetInFile := journalInode.I_size // Offset lógico dentro del archivo
	entrySize := int32(binary.Size(structures.Journal{}))

	// Encontrar bloque físico y offset dentro del bloque
	targetBlockLogicalIndex := writeOffsetInFile / sb.S_block_size
	offsetInTargetBlock := writeOffsetInFile % sb.S_block_size

	// Verificar si cabe en el bloque actual o necesita uno nuevo (simplificado: error si cruza)
	if offsetInTargetBlock+entrySize > sb.S_block_size {
		fmt.Printf("    Advertencia: Nueva entrada journal cruzaría límite de bloque (offset %d + size %d > blocksize %d). No soportado.\n", offsetInTargetBlock, entrySize, sb.S_block_size)
		return fmt.Errorf("journal lleno o entrada cruza límite de bloque (no soportado)")
	}

	if targetBlockLogicalIndex >= 12 { // Asumiendo solo punteros directos para el journal por ahora
		fmt.Printf("    Advertencia: Journal excede punteros directos (índice lógico %d). No soportado.\n", targetBlockLogicalIndex)
		return fmt.Errorf("journal lleno (excede punteros directos)")
	}

	physicalBlockIndex := journalInode.I_block[targetBlockLogicalIndex]
	if physicalBlockIndex == -1 {
		// TODO?: Implementar asignación de nuevo bloque si el puntero es -1
		fmt.Printf("    Advertencia: Journal necesita bloque lógico %d pero puntero es -1. No soportado.\n", targetBlockLogicalIndex)
		return fmt.Errorf("journal lleno o inconsistente (puntero -1)")
	}
	if physicalBlockIndex < 0 || physicalBlockIndex >= sb.S_blocks_count {
		return fmt.Errorf("appendToJournal: puntero inválido %d en journal inode block[%d]", physicalBlockIndex, targetBlockLogicalIndex)
	}

	// Calcular offset físico absoluto en el disco
	physicalWriteOffset := int64(sb.S_block_start) + int64(physicalBlockIndex)*int64(sb.S_block_size) + int64(offsetInTargetBlock)

	// Crear la entrada completa del Journal
	journalEntry := structures.Journal{
		J_count:   writeOffsetInFile/entrySize + 1, // Estimación simple del número de entrada
		J_content: entryData,                       // Los datos vienen como parámetro
	}
	journalEntry.J_content.I_date = float32(time.Now().Unix()) // Poner fecha actual

	// Escribir la entrada en el disco
	fmt.Printf("    Escribiendo entrada journal en offset físico %d (offset lógico %d)\n", physicalWriteOffset, writeOffsetInFile)
	file, errOpen := os.OpenFile(diskPath, os.O_WRONLY, 0644) // Abrir solo para escribir
	if errOpen != nil {
		return fmt.Errorf("appendToJournal: error abriendo disco para escribir journal: %w", errOpen)
	}
	defer file.Close()

	_, errSeek := file.Seek(physicalWriteOffset, 0)
	if errSeek != nil {
		file.Close()
		return fmt.Errorf("appendToJournal: error buscando offset %d: %w", physicalWriteOffset, errSeek)
	}
	errWrite := binary.Write(file, binary.LittleEndian, &journalEntry)
	if errWrite != nil {
		file.Close()
		return fmt.Errorf("appendToJournal: error escribiendo entrada: %w", errWrite)
	}

	// Actualizar tamaño y mtime del inodo del journal
	journalInode.I_size += entrySize
	journalInode.I_mtime = float32(time.Now().Unix())
	// No necesitamos atime aquí

	// Serializar inodo del journal actualizado
	fmt.Println("    Actualizando inodo del journal...")
	errSer := journalInode.Serialize(diskPath, journalInodeOffset)
	if errSer != nil {
		return fmt.Errorf("appendToJournal: error crítico guardando inodo journal actualizado: %w", errSer)
	}
	fmt.Println("--> appendToJournal: Entrada añadida exitosamente.")
	return nil
}


// Función auxiliar para copiar string a array de bytes 
func StringToBytes(str string, size int) [ ]byte {
	bytes := make([]byte, size)
	copy(bytes, str)
	return bytes
}

// Sobreescribe stringToBytes para tipos específicos si es necesario
func StringToBytes10(str string) [10]byte { b := [10]byte{}; copy(b[:], str); return b }
func StringToBytesN(str string, n int) []byte { buf := make([]byte, n); copy(buf, str); return buf }
// Helper específico para [32]byte de path
func StringToBytes32(str string) [32]byte { b := [32]byte{}; copy(b[:], str); return b }
// Helper específico para [64]byte de content
func StringToBytes64(str string) [64]byte { b := [64]byte{}; copy(b[:], str); return b }
