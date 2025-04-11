package commands

import (
	stores "backend/stores"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type RMDISK struct {
	path string // Path del disco
}

func ParseRmdisk(tokens []string) (string, error) {
	cmd := &RMDISK{} // Crea una nueva instancia de RMDISK

	// Unir tokens en una sola cadena y luego dividir por espacios, respetando las comillas
	args := strings.Join(tokens, " ")
	// Expresión regular para encontrar los parámetros del comando mkdir
	re := regexp.MustCompile(`-path=[^\s]+|-p`)
	// Encuentra todas las coincidencias de la expresión regular en la cadena de argumentos
	matches := re.FindAllString(args, -1)

	// Verificar que todos los tokens fueron reconocidos por la expresión regular
	if len(matches) != len(tokens) {
		// Identificar el parámetro inválido
		for _, token := range tokens {
			if !re.MatchString(token) {
				return "", fmt.Errorf("parámetro inválido: %s", token)
			}
		}
	}

	// Itera sobre cada coincidencia encontrada
	for _, match := range matches {
		// Divide cada parte en clave y valor usando "=" como delimitador
		kv := strings.SplitN(match, "=", 2)
		key := strings.ToLower(kv[0])

		// Switch para manejar diferentes parámetros
		switch key {
		case "-path":
			if len(kv) != 2 {
				return "", fmt.Errorf("formato de parámetro inválido: %s", match)
			}
			value := kv[1]
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = strings.Trim(value, "\"")
			}
			cmd.path = value
		default:
			// Si el parámetro no es reconocido, devuelve un error
			return "", fmt.Errorf("parámetro desconocido: %s", key)
		}
	}

	// Verifica que el parámetro -path haya sido proporcionado
	if cmd.path == "" {
		return "", errors.New("faltan parámetros requeridos: -path")
	}

	// Aquí se puede agregar la lógica para ejecutar el comando mkdir con los parámetros proporcionados
	err := commandRmdisk(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Rmdisk: Disco %s eliminado exitosamente.", cmd.path), nil // Devuelve el comando MKDIR creado

}

func commandRmdisk(rmdisk *RMDISK) error {

	if _, err := os.Stat(rmdisk.path); os.IsNotExist(err) {
		return fmt.Errorf("no existe el archivo de disco '%s'", rmdisk.path)
	}

	//  VERIFICAR SI HAY PARTICIONES MONTADAS DE ESTE DISCO
	mountedFromThisDisk := []string{}
	for id, mountedPath := range stores.MountedPartitions {
		// Comparar paths después de limpiarlos por si acaso
		if filepath.Clean(mountedPath) == filepath.Clean(rmdisk.path) {
			mountedFromThisDisk = append(mountedFromThisDisk, id)
		}
	}
	if len(mountedFromThisDisk) > 0 {
		return fmt.Errorf("error: no se puede eliminar el disco '%s' porque las siguientes particiones están montadas: %v", rmdisk.path, mountedFromThisDisk)
	}

	// Intentar eliminar el archivo físico
	fmt.Printf("Intentando eliminar archivo físico: %s\n", rmdisk.path)
	err := os.Remove(rmdisk.path)
	if err != nil {
		return fmt.Errorf("error al eliminar el archivo '%s': %w", rmdisk.path, err)
	}
	fmt.Printf("Archivo de disco %s eliminado exitosamente del sistema.\n", rmdisk.path)

	//  Quitar del Registro de Discos 
	if _, exists := stores.DiskRegistry[rmdisk.path]; exists {
		delete(stores.DiskRegistry, rmdisk.path)
		fmt.Printf("Disco '%s' eliminado del registro.\n", rmdisk.path)
	} else {
		fmt.Printf("Advertencia: Disco '%s' no encontrado en el registro para eliminar.\n", rmdisk.path)
	}
	
	return nil
}
