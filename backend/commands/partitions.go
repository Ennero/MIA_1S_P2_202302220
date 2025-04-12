package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
	diskPath := cmd.path
	diskBaseName := filepath.Base(diskPath)
	fmt.Printf("Buscando particiones para disco: '%s' (%s)\n", diskBaseName, diskPath)

	var mbr structures.MBR
	err := mbr.Deserialize(diskPath)
	if err != nil {
		return "", fmt.Errorf("error leyendo MBR '%s': %w", diskPath, err)
	}

	var validPartitionStrings []string
	validPartitionsForSort := []structures.Partition{}
	for _, p := range mbr.Mbr_partitions {
		if p.Part_size > 0 && p.Part_status[0] != 'N' && p.Part_status[0] != 0 {
			validPartitionsForSort = append(validPartitionsForSort, p)
		}
	}
	sort.Slice(validPartitionsForSort, func(i, j int) bool {
		return validPartitionsForSort[i].Part_start < validPartitionsForSort[j].Part_start
	})

	for _, p := range validPartitionsForSort {
		partName := strings.TrimRight(string(p.Part_name[:]), "\x00 ")
		if partName == "" {
			partName = "[Sin Nombre]"
		}
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

		mountIdStr := ""
		if foundId, isMounted := stores.GetMountIDForPartition(diskPath, partName); isMounted {
			mountIdStr = foundId
			fmt.Printf("  Partición '%s' está montada con ID: %s\n", partName, mountIdStr)
		}

		// Formato
		partitionStr := fmt.Sprintf("%s,%c,%d,%d,%c,%c,%s",
			partName, partType, partSize, partStart, partFit, partStatus, mountIdStr,
		)
		validPartitionStrings = append(validPartitionStrings, partitionStr)

		// Si es extendida, buscar y añadir lógicas
		if partType == 'E' {
			errLogic := addLogicalPartitionsInfo(diskPath, p.Part_start, &validPartitionStrings) // Pasamos diskPath
			if errLogic != nil {
				fmt.Printf("Advertencia: Error leyendo particiones lógicas de '%s': %v\n", diskPath, errLogic)
			}
		}
	}

	output := strings.Join(validPartitionStrings, ";")
	return output, nil
}

func addLogicalPartitionsInfo(diskPath string, extendedStart int32, resultStrings *[]string) error {
	file, err := os.Open(diskPath)
	if err != nil {
		return fmt.Errorf("error abriendo disco lógicas: %w", err)
	}
	defer file.Close()
	currentPos := int64(extendedStart)
	fmt.Printf("Buscando lógicas desde: %d\n", currentPos)

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
			if ebr.Part_next == -1 {
				break
			}
			if int64(ebr.Part_next) <= currentPos {
				return fmt.Errorf("ciclo EBR(1)")
			}
			currentPos = int64(ebr.Part_next)
			continue
		}

		partName := strings.TrimRight(string(ebr.Part_name[:]), "\x00 ")
		if partName == "" {
			partName = "[Sin Nombre Lógica]"
		}
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

		mountIdStr := ""
		// Usamos la misma función helper pasando el diskPath y el nombre de la lógica
		if foundId, isMounted := stores.GetMountIDForPartition(diskPath, partName); isMounted {
			mountIdStr = foundId
			fmt.Printf("  Partición Lógica '%s' está montada con ID: %s\n", partName, mountIdStr)
		}

		// Formato
		partitionStr := fmt.Sprintf("%s,%c,%d,%d,%c,%c,%s",
			partName, partType, partSize, partStart, partFit, partStatus, mountIdStr,
		)
		*resultStrings = append(*resultStrings, partitionStr)
		fmt.Printf("  Lógica encontrada: %s\n", partitionStr)

		if ebr.Part_next == -1 {
			break
		}
		if int64(ebr.Part_next) <= currentPos {
			return fmt.Errorf("ciclo EBR(2)")
		}
		currentPos = int64(ebr.Part_next)
	}
	return nil
}
