package commands

import (
	"encoding/binary" // Necesario para addLogicalPartitionsInfo
	"errors"
	"fmt"
	"os"            // Necesario para validar si el archivo existe
	"path/filepath" // Para obtener nombre base si se necesitara
	"regexp"
	"sort"
	"strings"
	structures "backend/structures"
)

type PARTITIONS struct {
	path string 
}

func ParsePartitions(tokens []string) (string, error) {
	cmd := &PARTITIONS{}
	processedKeys := make(map[string]bool)

	// Regex para -path=<valor>
	pathRegex := regexp.MustCompile(`^(?i)-path=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -path=<ruta_disco>")
	}

	// Este comando solo debe tener el parámetro -path
	if len(tokens) > 1 {
		return "", fmt.Errorf("demasiados parámetros para partitions, solo se espera -path=<ruta_disco>")
	}
	token := strings.TrimSpace(tokens[0])
	if token == "" {
		return "", errors.New("parámetro vacío proporcionado")
	}

	match := pathRegex.FindStringSubmatch(token)
	if match != nil {
		key := "-path"
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
			return "", errors.New("el valor para -path no puede estar vacío")
		}

		// Validar si el archivo de disco existe
		cleanedPath := filepath.Clean(value) 
		if _, err := os.Stat(cleanedPath); os.IsNotExist(err) {
			return "", fmt.Errorf("error: el archivo de disco especificado en -path no existe: '%s'", cleanedPath)
		} else if err != nil {
			return "", fmt.Errorf("error al verificar el archivo de disco '%s': %w", cleanedPath, err)
		}

		cmd.path = cleanedPath 

	} else {
		return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -path=<ruta_disco>", token)
	}

	if !processedKeys["-path"] {
		return "", errors.New("falta el parámetro requerido: -path")
	}

	// Llamar a la lógica del comando
	output, err := commandPartitions(cmd)
	if err != nil {
		return "", err
	}

	if output == "" {
		return fmt.Sprintf("PARTITIONS: No se encontraron particiones válidas en el disco '%s'.", cmd.path), nil
	}

	return "PARTITIONS:\n" + output, nil
}

func commandPartitions(cmd *PARTITIONS) (string, error) {
	diskPath := cmd.path                    // Usar el path directamente
	diskBaseName := filepath.Base(diskPath) // Obtener nombre base para mensajes
	fmt.Printf("Buscando particiones para el disco: '%s' (%s)\n", diskBaseName, diskPath)

	// Leer el MBR del disco especificado
	var mbr structures.MBR
	err := mbr.Deserialize(diskPath)
	if err != nil {
		return "", fmt.Errorf("error leyendo MBR del disco '%s': %w", diskPath, err)
	}

	// Recopilar y Formatear Información de Particiones Válidas
	var validPartitionStrings []string
	validPartitionsForSort := []structures.Partition{}

	// Recopilar particiones válidas del MBR
	for _, p := range mbr.Mbr_partitions {
		// Partición válida: tamaño > 0 y estado no 'N' (o 0)
		if p.Part_size > 0 && p.Part_status[0] != 'N' && p.Part_status[0] != 0 {
			validPartitionsForSort = append(validPartitionsForSort, p)
		}
	}

	sort.Slice(validPartitionsForSort, func(i, j int) bool {
		return validPartitionsForSort[i].Part_start < validPartitionsForSort[j].Part_start
	})

	for _, p := range validPartitionsForSort {
		partName := strings.TrimRight(string(p.Part_name[:]), "\x00 ")
		partType := p.Part_type[0]
		if partType == 0 {
			partType = ' '
		}
		partSize := p.Part_size
		partStart := p.Part_start
		partFit := p.Part_fit[0]
		if partFit == 0 {
			partFit = ' '
		}
		partStatus := p.Part_status[0] 

		// Formato: nombre,tipo,tamaño,inicio,fit,estado
		partitionStr := fmt.Sprintf("%s,%c,%d,%d,%c,%c",
			partName, partType, partSize, partStart, partFit, partStatus,
		)
		validPartitionStrings = append(validPartitionStrings, partitionStr)

		// Si es extendida, buscar y añadir lógicas
		if partType == 'E' {
			errLogic := addLogicalPartitionsInfo(diskPath, p.Part_start, &validPartitionStrings)
			if errLogic != nil {
				fmt.Printf("Advertencia: Error leyendo particiones lógicas de '%s': %v\n", diskPath, errLogic)
			}
		}

	} 

	// Unir las cadenas con punto y coma
	output := strings.Join(validPartitionStrings, ";")

	return output, nil
}

// Función auxiliar para leer EBRs y añadir info de lógicas
func addLogicalPartitionsInfo(diskPath string, extendedStart int32, resultStrings *[]string) error {
	file, err := os.Open(diskPath)
	if err != nil {
		return fmt.Errorf("error abriendo disco lógicas: %w", err)
	}
	defer file.Close()
	currentPos := int64(extendedStart)
	fmt.Printf("Buscando lógicas desde: %d\n", currentPos)
	ebrSize := int32(binary.Size(structures.EBR{})) 

	for {
		_, err = file.Seek(currentPos, 0)
		if err != nil {
			return fmt.Errorf("seek error EBR %d: %w", currentPos, err)
		}
		var ebr structures.EBR
		err = binary.Read(file, binary.LittleEndian, &ebr)
		if err != nil || (ebr.Part_size <= 0 && currentPos != int64(extendedStart)) {
			if err != nil && !errors.Is(err, errors.New("EOF")) {
				fmt.Printf("Error leyendo EBR %d: %v\n", currentPos, err)
			}
			break 
		}
		if ebr.Part_size <= 0 && currentPos == int64(extendedStart) {
			fmt.Println("  Saltando EBR contenedor inicial/vacío.")
			if ebr.Part_next == -1 {
				break
			} 
			if int64(ebr.Part_next) <= currentPos {
				return fmt.Errorf("ciclo EBR detectado (next=%d)", ebr.Part_next)
			}
			currentPos = int64(ebr.Part_next)
			continue
		}

		// Procesar EBR válido que representa una lógica
		partName := strings.TrimRight(string(ebr.Part_name[:]), "\x00 ")
		partType := byte('L')
		partSize := ebr.Part_size
		partStart := ebr.Part_start
		partFit := ebr.Part_fit[0]
		if partFit == 0 {
			partFit = ' '
		}
		partStatus := ebr.Part_status[0]
		if partStatus == 0 {
			partStatus = 'N'
		}

		// Validar start/size de la lógica contra el EBR
		if partStart != int32(currentPos)+ebrSize {
			fmt.Printf("Advertencia: Inicio de lógica (%d) no coincide con esperado (%d) para EBR en %d.\n", partStart, int32(currentPos)+ebrSize, currentPos)
		}

		partitionStr := fmt.Sprintf("%s,%c,%d,%d,%c,%c", partName, partType, partSize, partStart, partFit, partStatus)
		*resultStrings = append(*resultStrings, partitionStr)
		fmt.Printf("  Lógica encontrada: %s\n", partitionStr)

		if ebr.Part_next == -1 {
			break
		}
		if int64(ebr.Part_next) <= currentPos {
			return fmt.Errorf("ciclo EBR detectado (next=%d)", ebr.Part_next)
		}
		currentPos = int64(ebr.Part_next)
	}
	return nil
}
