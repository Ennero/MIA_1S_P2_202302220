package commands

import (
	stores "backend/stores"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type UNMOUNT struct {
	id string // ID de la partición a desmontar
}

func ParseUnmount(tokens []string) (string, error) {
	cmd := &UNMOUNT{}
	processedKeys := make(map[string]bool)

	//Valor entre comillas |Valor sin comillas
	idRegex := regexp.MustCompile(`^(?i)-id=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -id=<valor>")
	}

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		match := idRegex.FindStringSubmatch(token)
		if match != nil { 
			key := "-id"
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

			// Validar valor vacío
			if value == "" {
				return "", errors.New("el valor para -id no puede estar vacío")
			}

			cmd.id = value

		} else {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -id=<valor>", token)
		}
	}

	// Verificar que -id fue proporcionado
	if !processedKeys["-id"] {
		return "", errors.New("falta el parámetro requerido: -id")
	}

	// Validar formato de ID si es necesario 
	if !strings.HasPrefix(cmd.id, stores.Carnet) {
		fmt.Printf("Advertencia: El ID '%s' no parece tener el formato esperado (ej: %s1A).\n", cmd.id, stores.Carnet)
	}

	// Llamar a la lógica del comando
	err := commandUnmount(*cmd)
	if err != nil {
		return "", err 
	}

	return fmt.Sprintf("UNMOUNT: Partición con id '%s' desmontada exitosamente.", cmd.id), nil
}

func commandUnmount(cmd UNMOUNT) error {
	fmt.Printf("Intentando desmontar partición con ID: %s\n", cmd.id)

	// 1. Verificar si el ID está realmente montado en nuestro store
	diskPath, mounted := stores.MountedPartitions[cmd.id]
	if !mounted {
		return fmt.Errorf("error: la partición con id '%s' no se encuentra montada", cmd.id)
	}
	fmt.Printf("  Partición encontrada en disco: %s\n", diskPath)

	// Obtener el MBR y el puntero a la Partición en memoria
	mbr, partitionPtr, _, err := stores.GetMountedPartitionInfo(cmd.id)
	if err != nil {
		return fmt.Errorf("error crítico al obtener información de la partición '%s' desde el disco '%s': %w", cmd.id, diskPath, err)
	}

	partitionName := strings.TrimRight(string(partitionPtr.Part_name[:]), "\x00 ")

	// Modificar el estado de la partición en memoria 
	fmt.Printf("  Modificando estado de montaje para partición '%s' (ID: %s) en MBR...\n", partitionName, cmd.id)
	partitionPtr.Part_correlative = 0
	partitionPtr.Part_id = [4]byte{}  // Limpiar ID
	partitionPtr.Part_status[0] = '0' 
	fmt.Println("  Estado en memoria MBR modificado:")
	partitionPtr.PrintPartition()

	// Serializar el MBR modificado de vuelta al disco
	fmt.Println("  Serializando MBR actualizado al disco...")
	err = mbr.Serialize(diskPath)
	if err != nil {
		fmt.Printf("¡ERROR CRÍTICO! No se pudo guardar el MBR actualizado en '%s': %v\n", diskPath, err)
		fmt.Println("El estado de montaje en disco puede no haberse actualizado.")
		return fmt.Errorf("error fatal al guardar MBR actualizado para desmontaje: %w", err)
	}
	fmt.Println("  MBR guardado exitosamente.")

	// Eliminar la partición de los stores globales
	fmt.Printf("  Eliminando partición ID '%s' de stores globales...\n", cmd.id)
	delete(stores.MountedPartitions, cmd.id) // Quitar del mapa principal

	// Quitar ID de ListMounted
	foundIndex := -1
	for i, mountedID := range stores.ListMounted {
		if mountedID == cmd.id {
			foundIndex = i
			break
		}
	}
	if foundIndex != -1 {
		stores.ListMounted = slices.Delete(stores.ListMounted, foundIndex, foundIndex+1) // Go 1.21+
	} else {
		fmt.Printf("Advertencia: ID '%s' no encontrado en stores.ListMounted para eliminar.\n", cmd.id)
	}

	foundNameIndex := -1
	if partitionName != "" { // Solo intentar si obtuvimos un nombre
		for i, name := range stores.ListPatitions {
			if name == partitionName {
				foundNameIndex = i
				break
			}
		}
		if foundNameIndex != -1 {
			stores.ListPatitions = slices.Delete(stores.ListPatitions, foundNameIndex, foundNameIndex+1) // Go 1.21+
			fmt.Printf("  Nombre '%s' eliminado de stores.ListPatitions.\n", partitionName)
		} else {
			fmt.Printf("Advertencia: Nombre '%s' no encontrado en stores.ListPatitions para eliminar.\n", partitionName)
		}
	}

	fmt.Printf("  Stores actualizados. Montadas ahora: %v\n", stores.ListMounted)

	//Logout si era la partición activa
	if stores.Auth.IsAuthenticated() && stores.Auth.GetPartitionID() == cmd.id {
		fmt.Println("INFO: Se está desmontando la partición activa. Realizando logout automático.")
		stores.Auth.Logout()
	}

	fmt.Println("Desmontaje completado.")
	return nil
}