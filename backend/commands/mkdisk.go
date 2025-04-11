package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	utils "backend/utils"
	"errors"        // Paquete para manejar errores y crear nuevos errores con mensajes personalizados
	"fmt"           // Paquete para formatear cadenas y realizar operaciones de entrada/salida
	"math/rand"     // Paquete para generar números aleatorios
	"os"            // Paquete para interactuar con el sistema operativo
	"path/filepath" // Paquete para trabajar con rutas de archivos y directorios
	"regexp"        // Paquete para trabajar con expresiones regulares, útil para encontrar y manipular patrones en cadenas
	"strconv"       // Paquete para convertir cadenas a otros tipos de datos, como enteros
	"strings"       // Paquete para manipular cadenas, como unir, dividir, y modificar contenido de cadenas
	"time"
)

// MKDISK estructura que representa el comando mkdisk con sus parámetros
type MKDISK struct {
	size int    // Tamaño del disco
	unit string // Unidad de medida del tamaño (K o M)
	fit  string // Tipo de ajuste (BF, FF, WF)
	path string // Ruta del archivo del disco
}

func ParseMkdisk(tokens []string) (string, error) {
	cmd := &MKDISK{}
	foundParams := make(map[string]bool)

	originalInput := strings.Join(tokens, " ")
	args := originalInput

	re := regexp.MustCompile(`-size=\d+|-unit=[kKmM]|-fit=[bBfFwW]{2}|-path="[^"]+"|-path=[^\s]+`)
	matches := re.FindAllString(args, -1)

	tempArgs := args

	for _, match := range matches {
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("error interno al parsear formato clave=valor: '%s'", match)
		}
		key, value := strings.ToLower(kv[0]), kv[1]

		// Verificar duplicados ANTES de procesar
		if foundParams[key] {
			return "", fmt.Errorf("parámetro '%s' especificado más de una vez", key)
		}

		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		// Asignar valores y marcar como encontrado
		switch key {
		case "-size":
			size, err := strconv.Atoi(value)
			if err != nil || size <= 0 {
				return "a", errors.New("el tamaño (-size) debe ser un número entero positivo")
			}
			cmd.size = size
			foundParams[key] = true
		case "-unit":
			unitVal := strings.ToUpper(value)
			if unitVal != "K" && unitVal != "M" {
				return "a", errors.New("la unidad (-unit) debe ser K o M")
			}
			cmd.unit = unitVal
			foundParams[key] = true
		case "-fit":
			fitVal := strings.ToUpper(value)
			if fitVal != "BF" && fitVal != "FF" && fitVal != "WF" {
				return "a", errors.New("el ajuste (-fit) debe ser BF, FF o WF")
			}
			cmd.fit = fitVal
			foundParams[key] = true
		case "-path":
			if value == "" {
				return "a", errors.New("el path (-path) no puede estar vacío")
			}
			cmd.path = value
			foundParams[key] = true
		default:
			return "a", fmt.Errorf("clave de parámetro desconocida encontrada: %s", key)
		}

		tempArgs = strings.Replace(tempArgs, match, "", 1)

	}

	remainingInput := strings.TrimSpace(tempArgs)
	if remainingInput != "" {
		firstUnknown := remainingInput
		if spaceIndex := strings.Index(firstUnknown, " "); spaceIndex != -1 {
			firstUnknown = firstUnknown[:spaceIndex]
		}
		return "a", fmt.Errorf("parámetro o texto no reconocido cerca de: '%s'", firstUnknown)
	}

	if !foundParams["-size"] {
		return "a", errors.New("falta parámetro requerido: -size")
	}
	if !foundParams["-path"] {
		return "a", errors.New("falta parámetro requerido: -path")
	}

	if !foundParams["-unit"] {
		cmd.unit = "M"
	}
	if !foundParams["-fit"] {
		cmd.fit = "FF"
	}

	err := commandMkdisk(cmd)
	if err != nil {
		return "", fmt.Errorf("error al ejecutar mkdisk: %w", err)
	}

	return fmt.Sprintf("MKDISK: Disco creado exitosamente\n"+
		"-> Path: %s\n"+
		"-> Tamaño: %d%s\n"+
		"-> Fit: %s",
		cmd.path, cmd.size, cmd.unit, cmd.fit), nil
}


func commandMkdisk(mkdisk *MKDISK) error {

	// Crear directorio padre si no existe
	dir := filepath.Dir(mkdisk.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creando directorio padre '%s': %w", dir, err)
	}

	// Calcular tamaño en bytes
	sizeBytes, err := utils.ConvertToBytes(mkdisk.size, mkdisk.unit)
	if err != nil {
		return fmt.Errorf("error convirtiendo tamaño: %w", err)
	}

	// Crear archivo binario con ceros
	file, err := os.Create(mkdisk.path)
	if err != nil {
		return fmt.Errorf("error creando archivo de disco '%s': %w", mkdisk.path, err)
	}
	defer file.Close()

	// Escribir un byte nulo al inicio
	if _, err := file.Write([]byte{0}); err != nil {
		return fmt.Errorf("error escribiendo byte inicial en '%s': %w", mkdisk.path, err)
	}

	// Restar 1 porque el offset es 0-based
	offset := int64(sizeBytes) - 1
	if offset < 0 {
		offset = 0
	} 

	if _, err := file.Seek(offset, 0); err != nil {
		return fmt.Errorf("error buscando final del archivo en '%s': %w", mkdisk.path, err)
	}
	if _, err := file.Write([]byte{0}); err != nil { // Escribir byte nulo al final
		return fmt.Errorf("error escribiendo byte final en '%s': %w", mkdisk.path, err)
	}
	fmt.Printf("Archivo de disco '%s' creado/extendido a %d bytes.\n", mkdisk.path, sizeBytes)

	// Inicializar MBR 
	mbr := structures.MBR{
		Mbr_size:           int32(sizeBytes),
		Mbr_creation_date:  float32(time.Now().Unix()),
		Mbr_disk_signature: int32(rand.Intn(100000)), // Firma aleatoria simple
		Mbr_disk_fit:       [1]byte{mkdisk.fit[0]},   // Guardar fit seleccionado
	}
	// Inicializar particiones vacías
	for i := range mbr.Mbr_partitions {
		mbr.Mbr_partitions[i].Part_status[0] = 'N' // 'N' para No usada
		mbr.Mbr_partitions[i].Part_start = -1
		mbr.Mbr_partitions[i].Part_size = 0
	}

	// Serializar MBR al inicio del archivo
	if err := mbr.Serialize(mkdisk.path); err != nil { // Llama al método Serialize del MBR
		return fmt.Errorf("error escribiendo MBR inicial en '%s': %w", mkdisk.path, err)
	}
	fmt.Println("MBR inicializado y escrito en el disco.")

	//  Añadir al Registro de Discos 
	diskBaseName := filepath.Base(mkdisk.path)
	stores.DiskRegistry[mkdisk.path] = diskBaseName // Guardar path completo -> nombre base
	fmt.Printf("Disco '%s' (Path: '%s') añadido al registro.\n", diskBaseName, mkdisk.path)

	return nil
}
