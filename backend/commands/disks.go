// commands/disks.go
package commands

import (
	"fmt"
	"sort" // Para mostrar discos ordenados por path
	"strings"
	stores "backend/stores"
	structures "backend/structures"
)


func ParseDisks(tokens []string) (string, error) {
	if len(tokens) > 0 {
		return "", fmt.Errorf("el comando 'disks' no acepta parámetros, se encontró: %v", tokens)
	}

	// Llamar a la lógica del comando
	output, err := commandDisks()
	if err != nil {
		return "", err
	}

	return output, nil
}

func commandDisks() (string, error) {
	fmt.Println("Obteniendo información de los discos registrados...")

	if len(stores.DiskRegistry) == 0 {
		return "DISKS: No hay discos registrados en el sistema.", nil
	}

	// Obtener Paths y Ordenarlos 
	diskPaths := make([]string, 0, len(stores.DiskRegistry))
	for path := range stores.DiskRegistry {
		diskPaths = append(diskPaths, path)
	}
	sort.Strings(diskPaths) // Ordenar alfabéticamente por path

	// Preparar Salida 
	var outputBuilder strings.Builder
	firstDisk := true

	// Iterar sobre Discos Registrados 
	for _, diskPath := range diskPaths {
		diskName := stores.DiskRegistry[diskPath] // Obtener nombre base del registro
		fmt.Printf("Procesando disco: '%s' (%s)\n", diskName, diskPath)

		// Leer MBR del disco
		var mbr structures.MBR
		err := mbr.Deserialize(diskPath)
		if err != nil {
			fmt.Printf("  Advertencia: No se pudo leer MBR para disco '%s': %v. Saltando disco.\n", diskPath, err)
			continue 
		}

		// Extraer información del MBR
		diskSize := mbr.Mbr_size
		diskFit := mbr.Mbr_disk_fit[0]
		if diskFit == 0 {
			diskFit = ' '
		} 

		// Encontrar Particiones Montadas para ESTE disco
		mountedNames := []string{}
		for mountID, mountedDiskPath := range stores.MountedPartitions {
			if mountedDiskPath == diskPath {
				// Encontramos una partición montada de este disco, obtener su nombre
				part, errPart := mbr.GetPartitionByID(mountID) // Usar el MBR ya leído
				if errPart == nil && part != nil {
					partName := strings.TrimRight(string(part.Part_name[:]), "\x00 ")
					if partName != "" {
						mountedNames = append(mountedNames, partName)
					} else {
						mountedNames = append(mountedNames, fmt.Sprintf("[Sin Nombre, ID:%s]", mountID))
					}
				} else {
					mountedNames = append(mountedNames, fmt.Sprintf("[Error Partición ID:%s]", mountID))
				}
			}
		}
		mountedStr := "Ninguna"
		if len(mountedNames) > 0 {
			mountedStr = strings.Join(mountedNames, "|") // Unir con '|'
		}

		// Formatear la línea para este disco
		line := fmt.Sprintf("%s,%s,%d,%c,%s",
			diskName,
			diskPath,
			diskSize,
			diskFit,
			mountedStr,
		)

		// Añadir punto y coma si no es el primer disco
		if !firstDisk {
			outputBuilder.WriteString(";")
		}
		outputBuilder.WriteString(line)
		firstDisk = false

	}

	// Devolver el string final
	finalOutput := outputBuilder.String()
	if finalOutput == "" {
		return "DISKS: No se pudo obtener información de ningún disco registrado.", nil
	}
	return "DISKS:\n" + finalOutput, nil // Añadir prefijo
}
