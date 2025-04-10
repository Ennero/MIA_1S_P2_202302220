package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	utils "backend/utils"
	"errors" // Paquete para manejar errores y crear nuevos errores con mensajes personalizados
	"fmt"    // Paquete para formatear cadenas y realizar operaciones de entrada/salida
	"regexp" // Paquete para trabajar con expresiones regulares, útil para encontrar y manipular patrones en cadenas

	// Paquete para convertir cadenas a otros tipos de datos, como enteros
	"strings" // Paquete para manipular cadenas, como unir, dividir, y modificar contenido de cadenas
)

// MOUNT estructura que representa el comando mount con sus parámetros
type MOUNT struct {
	path string // Ruta del archivo del disco
	name string // Nombre de la partición
}


// CommandMount parsea el comando mount y devuelve una instancia de MOUNT
func ParseMount(tokens []string) (string, error) {
	cmd := &MOUNT{} // Crea una nueva instancia de MOUNT

	// Unir tokens en una sola cadena y luego dividir por espacios, respetando las comillas
	args := strings.Join(tokens, " ")
	// Expresión regular para encontrar los parámetros del comando mount
	re := regexp.MustCompile(`-path="[^"]+"|-path=[^\s]+|-name="[^"]+"|-name=[^\s]+`)
	// Encuentra todas las coincidencias de la expresión regular en la cadena de argumentos
	matches := re.FindAllString(args, -1)

	// Itera sobre cada coincidencia encontrada
	for _, match := range matches {
		// Divide cada parte en clave y valor usando "=" como delimitador
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		key, value := strings.ToLower(kv[0]), kv[1]

		// Remove quotes from value if present
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		// Switch para manejar diferentes parámetros
		switch key {
		case "-path":
			// Verifica que el path no esté vacío
			if value == "" {
				return "", errors.New("el path no puede estar vacío")
			}
			cmd.path = value
		case "-name":
			// Verifica que el nombre no esté vacío
			if value == "" {
				return "", errors.New("el nombre no puede estar vacío")
			}
			cmd.name = value
		default:
			// Si el parámetro no es reconocido, devuelve un error
			return "", fmt.Errorf("parámetro desconocido: %s", key)
		}
	}

	// Verifica que los parámetros -path y -name hayan sido proporcionados
	if cmd.path == "" {
		return "", errors.New("faltan parámetros requeridos: -path")
	}
	if cmd.name == "" {
		return "", errors.New("faltan parámetros requeridos: -name")
	}

	// Montamos la partición
	err := commandMount(cmd)
	if err != nil {
		return "", err
	}

	if len(stores.ListMounted) == 0 {
		return "", errors.New("no hay particiones montadas")
	}
	lastElement := stores.ListMounted[len(stores.ListMounted)-1]



	// Devuelve un mensaje de éxito con los detalles del montaje
	return fmt.Sprintf("MOUNT: Partición montada exitosamente\n"+
		"-> Path: %s\n"+
		"-> Nombre: %s\n"+
		"-> ID: %s",
		cmd.path, cmd.name, lastElement), nil
}


func commandMount(mount *MOUNT) error {
	var mbr structures.MBR

	// Deserializar la estructura MBR desde un archivo binario
	err := mbr.Deserialize(mount.path)
	if err != nil {
		// Añadir más contexto al error
		return fmt.Errorf("error leyendo MBR del disco '%s': %w", mount.path, err)
	}

	// --- INICIO CORRECCIÓN ---
	// Buscar la partición con el nombre especificado, capturando el error
	partition, _, errFind := mbr.GetPartitionByName(mount.name) // <-- Recibe 3 valores

	// Verificar el error devuelto por GetPartitionByName
	if errFind != nil {
		// El error ya indica que no se encontró o hubo otro problema
		fmt.Printf("Error buscando partición '%s': %v\n", mount.name, errFind)
		return errFind // Devolver el error específico
	}

	/* SOLO PARA VERIFICACIÓN */
	// Print para verificar que la partición se encontró correctamente
	fmt.Println("\nPartición encontrada para montar:")
	partition.PrintPartition() // Usar el partition encontrado

	for _, valor := range stores.ListPatitions { 
		if valor == mount.name {
			fmt.Printf("Advertencia: Ya existe una partición montada con el nombre '%s' (puede ser de otro disco).\n", mount.name)
            return fmt.Errorf("ya existe una partición montada con el nombre '%s'", mount.name)
		}
	}

	// Generar un id único para la partición
	idPartition, partitionCorrelative, errGenID := generatePartitionID(mount)
	if errGenID != nil {
		fmt.Println("Error generando el id de partición:", errGenID)
		return errGenID
	}
	fmt.Printf("ID de montaje generado: %s (Correlativo: %d)\n", idPartition, partitionCorrelative)


	// Guardar la partición montada en la lista de montajes globales
	stores.MountedPartitions[idPartition] = mount.path
	stores.ListPatitions = append(stores.ListPatitions, mount.name) // ¿Realmente necesario guardar solo nombre?
	stores.ListMounted = append(stores.ListMounted, idPartition)
	fmt.Printf("Partición añadida a stores. Montadas ahora: %v\n", stores.ListMounted)


	partition.MountPartition(partitionCorrelative, idPartition) // Asumiendo que MountPartition existe en Partition

	/* SOLO PARA VERIFICACIÓN */
	fmt.Println("\nPartición marcada como montada (en memoria MBR):")
	partition.PrintPartition()


	// Serializar la estructura MBR completa (con la partición modificada)
	fmt.Println("Serializando MBR con estado de montaje actualizado...")
	err = mbr.Serialize(mount.path)
	if err != nil {
		// Si falla la serialización, el estado de montaje no se guarda en disco
		// Podríamos intentar revertir los cambios en 'stores'? Complicado.
		fmt.Println("Error serializando el MBR:", err)
		return fmt.Errorf("error serializando MBR con estado de montaje: %w", err)
	}

	fmt.Println("Montaje completado y MBR guardado.")
	return nil
}



func generatePartitionID(mount *MOUNT) (string, int, error) {
	// Asignar una letra a la partición y obtener el índice
	letter, partitionCorrelative, err := utils.GetLetterAndPartitionCorrelative(mount.path)
	if err != nil {
		fmt.Println("Error obteniendo la letra:", err)
		return "", 0, err
	}

	// Crear id de partición
	idPartition := fmt.Sprintf("%s%d%s", stores.Carnet, partitionCorrelative, letter)

	return idPartition, partitionCorrelative, nil
}
