// En el archivo reports/report_file.go
package reports

import (
	"backend/structures"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReportFile genera un reporte con el contenido de un archivo del sistema ext2
func ReportFile(superblock *structures.SuperBlock, diskPath string, outputPath string, filePath string) error {
	// Asegurar que el filePath sea absoluto
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}
	// Buscar el inodo del archivo
	_, inode, err := structures.FindInodeByPath(superblock, diskPath, filePath)
	if err != nil {
		return fmt.Errorf("error al buscar el inodo: %v", err)
	}
	if inode == nil {
		return fmt.Errorf("no se encontró el archivo en '%s'", filePath)
	}
	// Verificar que sea un archivo regular
	if inode.I_type[0] != '1' {
		return fmt.Errorf("'%s' no es un archivo regular", filePath)
	}
	// Leer contenido del archivo
	content, err := structures.ReadFileContent(superblock, diskPath, inode)
	if err != nil {
		return fmt.Errorf("error al leer el contenido: %v", err)
	}
	// Crear directorios de salida si no existen
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error al crear directorios de salida: %v", err)
	}
	// Escribir el contenido en el archivo de reporte
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("error al escribir el reporte: %v", err)
	}

	return nil
}