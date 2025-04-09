package commands

import (
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

/*
    mkdisk -size=3000 -unit=K -path=/home/user/Disco1.mia
    mkdisk -size=3000 -path=/home/user/Disco1.mia
    mkdisk -size=5 -unit=M -fit=WF -path="/home/keviin/University/PRACTICAS/MIA_LAB_S2_2024/CLASE03/disks/Disco1.mia"
    mkdisk -size=10 -path="/home/mis discos/Disco4.mia"
*/

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
	// Convertir el tamaño a bytes
	sizeBytes, err := utils.ConvertToBytes(mkdisk.size, mkdisk.unit)
	if err != nil {
		fmt.Println("Error converting size:", err)
		return err
	}

	// Crear el disco con el tamaño proporcionado
	err = createDisk(mkdisk, sizeBytes)
	if err != nil {
		fmt.Println("Error creating disk:", err)
		return err
	}

	// Crear el MBR con el tamaño proporcionado
	err = createMBR(mkdisk, sizeBytes)
	if err != nil {
		fmt.Println("Error creating MBR:", err)
		return err
	}

	return nil
}

func createDisk(mkdisk *MKDISK, sizeBytes int) error {
	// Crear las carpetas necesarias
	err := os.MkdirAll(filepath.Dir(mkdisk.path), os.ModePerm)
	if err != nil {
		fmt.Println("Error creating directories:", err)
		return err
	}

	// Crear el archivo binario
	file, err := os.Create(mkdisk.path)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return err
	}
	defer file.Close()

	// Escribir en el archivo usando un buffer de 1 MB
	buffer := make([]byte, 1024*1024) // Crea un buffer de 1 MB
	for sizeBytes > 0 {
		writeSize := len(buffer)
		if sizeBytes < writeSize {
			writeSize = sizeBytes // Ajusta el tamaño de escritura si es menor que el buffer
		}
		if _, err := file.Write(buffer[:writeSize]); err != nil {
			return err // Devuelve un error si la escritura falla
		}
		sizeBytes -= writeSize // Resta el tamaño escrito del tamaño total
	}
	return nil
}

func createMBR(mkdisk *MKDISK, sizeBytes int) error {
	// Seleccionar el tipo de ajuste
	var fitByte byte
	switch mkdisk.fit {
	case "FF":
		fitByte = 'F'
	case "BF":
		fitByte = 'B'
	case "WF":
		fitByte = 'W'
	default:
		fmt.Println("Invalid fit type")
		return nil
	}

	// Crear el MBR con los valores proporcionados
	mbr := &structures.MBR{
		Mbr_size:           int32(sizeBytes),
		Mbr_creation_date:  float32(time.Now().Unix()),
		Mbr_disk_signature: rand.Int31(),
		Mbr_disk_fit:       [1]byte{fitByte},
		Mbr_partitions: [4]structures.Partition{
			// Inicializó todos los char en N y los enteros en -1 para que se puedan apreciar en el archivo binario.

			{Part_status: [1]byte{'N'}, Part_type: [1]byte{'N'}, Part_fit: [1]byte{'N'}, Part_start: -1, Part_size: -1, Part_name: [16]byte{'N'}, Part_correlative: -1, Part_id: [4]byte{'N'}},
			{Part_status: [1]byte{'N'}, Part_type: [1]byte{'N'}, Part_fit: [1]byte{'N'}, Part_start: -1, Part_size: -1, Part_name: [16]byte{'N'}, Part_correlative: -1, Part_id: [4]byte{'N'}},
			{Part_status: [1]byte{'N'}, Part_type: [1]byte{'N'}, Part_fit: [1]byte{'N'}, Part_start: -1, Part_size: -1, Part_name: [16]byte{'N'}, Part_correlative: -1, Part_id: [4]byte{'N'}},
			{Part_status: [1]byte{'N'}, Part_type: [1]byte{'N'}, Part_fit: [1]byte{'N'}, Part_start: -1, Part_size: -1, Part_name: [16]byte{'N'}, Part_correlative: -1, Part_id: [4]byte{'N'}},
		},
	}

	/* SOLO PARA VERIFICACIÓN */
	// Imprimir MBR
	fmt.Println("\nMBR creado:")
	mbr.PrintMBR()

	// Serializar el MBR en el archivo
	err := mbr.Serialize(mkdisk.path)
	if err != nil {
		fmt.Println("Error:", err)
	}
	return nil
}
