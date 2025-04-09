package commands

import (
	structures "backend/structures"
	utils "backend/utils"
	"encoding/binary"
	"errors" // Paquete para manejar errores y crear nuevos errores con mensajes personalizados
	"fmt"    // Paquete para formatear cadenas y realizar operaciones de entrada/salida
	"os"
	"path/filepath"
	"regexp"  // Paquete para trabajar con expresiones regulares, útil para encontrar y manipular patrones en cadenas
	"strconv" // Paquete para convertir cadenas a otros tipos de datos, como enteros
	"strings" // Paquete para manipular cadenas, como unir, dividir, y modificar contenido de cadenas
)

// FDISK estructura que representa el comando fdisk con sus parámetros
type FDISK struct {
	size int    // Tamaño de la partición
	unit string // Unidad de medida del tamaño (K o M)
	fit  string // Tipo de ajuste (BF, FF, WF)
	path string // Ruta del archivo del disco
	typ  string // Tipo de partición (P, E, L)
	name string // Nombre de la partición
}

// ParseFdisk parsea el comando fdisk
func ParseFdisk(tokens []string) (string, error) {
	cmd := &FDISK{} 

	args := strings.Join(tokens, " ")

	re := regexp.MustCompile(`-size=(\d+)|-unit=([kKmMbB])|-fit=([bBfFwW]{2})|-path="([^"]+)"|-path=([^\s]+)|-type=([pPeElL])|-name="([^"]+)"|-name=([^\s]+)`)


	allMatches := re.FindAllStringSubmatch(args, -1)

	params := make(map[string]string)

	for _, match := range allMatches {
		if match[1] != "" { // -size
			params["-size"] = match[1]
		} else if match[2] != "" {
			params["-unit"] = match[2]
		} else if match[3] != "" { 
			params["-fit"] = match[3]
		} else if match[4] != "" {
			params["-path"] = match[4]
		} else if match[5] != "" { 
			params["-path"] = match[5]
		} else if match[6] != "" { 
			params["-type"] = match[6]
		} else if match[7] != "" {
			params["-name"] = match[7]
		} else if match[8] != "" {
			params["-name"] = match[8]
		}
	}
	// Procesar los parámetros almacenados
	for key, value := range params {
		cleanKey := strings.ToLower(key)


		switch cleanKey {
		case "-size":
			size, err := strconv.Atoi(value)
			if err != nil || size <= 0 {
				return "", fmt.Errorf("valor inválido para -size: '%s'. Debe ser un número entero positivo", value)
			}
			cmd.size = size
		case "-unit":
			unitUpper := strings.ToUpper(value)
			// Verifica que la unidad sea "K", "M" o "B"
			if unitUpper != "K" && unitUpper != "M" && unitUpper != "B" {
				return "", fmt.Errorf("valor inválido para -unit: '%s'. La unidad debe ser K, M o B", value)
			}
			cmd.unit = unitUpper // Almacena la unidad en mayúsculas
		case "-fit":
			fitUpper := strings.ToUpper(value)
			if fitUpper != "BF" && fitUpper != "FF" && fitUpper != "WF" {
				return "", fmt.Errorf("valor inválido para -fit: '%s'. El ajuste debe ser BF, FF o WF", value)
			}
			cmd.fit = fitUpper
		case "-path":
			if value == "" {
				return "", errors.New("el valor para -path no puede estar vacío") // Aunque regex debería prevenir esto
			}
			cmd.path = value
		case "-type":
			typeUpper := strings.ToUpper(value)
			if typeUpper != "P" && typeUpper != "E" && typeUpper != "L" {
				return "", fmt.Errorf("valor inválido para -type: '%s'. El tipo debe ser P, E o L", value)
			}
			cmd.typ = typeUpper
		case "-name":
			if value == "" {
				return "", errors.New("el valor para -name no puede estar vacío") // Aunque regex debería prevenir esto
			}
			cmd.name = value
		default:
			return "", fmt.Errorf("lógica de procesamiento faltante para parámetro: %s", key)
		}
	}

	if cmd.size == 0 {
		return "", errors.New("parámetro requerido faltante o inválido: -size")
	}
	if cmd.path == "" {
		return "", errors.New("parámetro requerido faltante: -path")
	}
	if cmd.name == "" {
		return "", errors.New("parámetro requerido faltante: -name")
	}

	// --- Establecer valores por defecto ---
	if cmd.unit == "" {
		cmd.unit = "M"
	}
	if cmd.fit == "" {
		cmd.fit = "FF" 
	}
	if cmd.typ == "" {
		cmd.typ = "P"
	}

	fmt.Println("Parámetros parseados:", cmd) 

	fmt.Println("Tamaño numérico ingresado:", cmd.size)
	fmt.Println("Unidad ingresada/default:", cmd.unit)

	filepath := filepath.Clean(cmd.path)

	var diskSize int64
	fileInfo, err := os.Stat(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Advertencia: El archivo del disco no existe, simulando tamaño de 100MB para validación.")
			return "", fmt.Errorf("no existe el disco")
		} else {
			fmt.Println("Error al obtener la información del disco:", err)
			return "", fmt.Errorf("no se pudo acceder a la información del disco en '%s': %w", filepath, err)
		}
	} else {
		diskSize = fileInfo.Size()
	}

	var partSizeInBytes int64
	if cmd.unit == "B" {
		// Si la unidad es Bytes, el tamaño ya está en bytes
		partSizeInBytes = int64(cmd.size)
	} else {
		// Si es K o M, convertir a bytes usando la función utilitaria
		sizeBytes, errConv := utils.ConvertToBytes(cmd.size, cmd.unit)
		if errConv != nil {
			fmt.Println("Error inesperado convirtiendo tamaño:", errConv)
			return "", errConv
		}
		partSizeInBytes = int64(sizeBytes)
	}

	fmt.Println("Tamaño de la partición calculado (bytes):", partSizeInBytes)
	fmt.Println("Tamaño total del disco (bytes):", diskSize)

	if partSizeInBytes > diskSize {
		return "", fmt.Errorf("el tamaño de la partición (%d bytes) no puede ser mayor que el tamaño del disco (%d bytes)", partSizeInBytes, diskSize)
	}
	if partSizeInBytes <= 0 {
		return "", fmt.Errorf("el tamaño calculado de la partición debe ser mayor que cero (%d bytes)", partSizeInBytes)
	}

	// Crear la partición con los parámetros proporcionados
	err = commandFdisk(cmd)
	if err != nil {
		fmt.Println("Error durante la ejecución de fdisk:", err)
		return "", fmt.Errorf("falló la creación de la partición: %w", err)
	}

	return fmt.Sprintf("FDISK: Partición creada exitosamente\n"+
		"-> Path: %s\n"+
		"-> Nombre: %s\n"+
		"-> Tamaño: %d%s\n"+ 
		"-> Tipo: %s\n"+
		"-> Fit: %s",
		cmd.path, cmd.name, cmd.size, cmd.unit, cmd.typ, cmd.fit), nil
}

func commandFdisk(fdisk *FDISK) error {
	// Convertir el tamaño a bytes
	sizeBytes, err := utils.ConvertToBytes(fdisk.size, fdisk.unit)
	if err != nil {
		fmt.Println("Error convirtiendo tamaño:", err)
		return err
	}

	switch fdisk.typ {
	case "P":
		// Crear partición primaria
		err = createPrimaryPartition(fdisk, sizeBytes)
		if err != nil {
			fmt.Println("Error creando partición primaria:", err)
			return err
		}
	case "E":
		// Crear partición extendida
		err = createExtendedPartition(fdisk, sizeBytes)
		if err != nil {
			fmt.Println("Error creando partición primaria:", err)
			return err
		}
	case "L":
		// Crear partición lógica
		err = createLogicalPartition(fdisk, sizeBytes)
		if err != nil {
			fmt.Println("Error creando partición primaria:", err)
			return err
		}
	}
	if err != nil {
		fmt.Println("Error creando partición:", err)
		return err
	}

	return nil
}

func createPrimaryPartition(fdisk *FDISK, sizeBytes int) error {
	// Crear una instancia de MBR
	var mbr structures.MBR

	// Deserializar la estructura MBR desde un archivo binario
	err := mbr.Deserialize(fdisk.path)
	if err != nil {
		fmt.Println("Error deserializando el MBR:", err)
		return fmt.Errorf("error deserializando el MBR: %w", err)
	}

	/* SOLO PARA VERIFICACIÓN */
	// Imprimir MBR
	fmt.Println("\nMBR original:")
	mbr.PrintMBR()

	// Obtener la primera partición disponible
	availablePartition, startPartition, indexPartition := mbr.GetFirstAvailablePartition()
	if availablePartition == nil {
		fmt.Println("No hay particiones disponibles.")
		return errors.New("no hay espacio disponible para la partición")
	}

	for _, partitionName := range mbr.GetPartitionNames() {
		if partitionName == fdisk.name {
			fmt.Println("Ya existe una partición con el nombre especificado.")
			return errors.New("ya existe una partición con el nombre especificado")
		}
	}

	/* SOLO PARA VERIFICACIÓN */
	// Print para verificar que la partición esté disponible
	fmt.Println("\nPartición disponible:")
	availablePartition.PrintPartition()

	// Crear la partición con los parámetros proporcionados
	availablePartition.CreatePartition(startPartition, sizeBytes, fdisk.typ, fdisk.fit, fdisk.name)

	// Print para verificar que la partición se haya creado correctamente
	fmt.Println("\nPartición creada (modificada):")
	availablePartition.PrintPartition()

	// Colocar la partición en el MBR
	mbr.Mbr_partitions[indexPartition] = *availablePartition

	// Imprimir las particiones del MBR
	fmt.Println("\nParticiones del MBR:")
	mbr.PrintPartitions()

	// Serializar el MBR en el archivo binario
	err = mbr.Serialize(fdisk.path)
	if err != nil {
		fmt.Println("Error:", err)
		return fmt.Errorf("error serializando el MBR: %w", err)
	}
	return nil
}

// Función para crear una partición extendida
func createExtendedPartition(fdisk *FDISK, sizeBytes int) error {
	var mbr structures.MBR

	// Deserializar el MBR del disco
	err := mbr.Deserialize(fdisk.path)
	if err != nil {
		fmt.Println("Error deserializando el MBR:", err)
		return fmt.Errorf("error deserializando el MBR: %w", err)
	}

	// Verificar si ya existe una partición extendida
	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_type[0] == 'E' {
			return errors.New("ya existe una partición extendida en el disco")
		}
	}

	// Obtener la primera partición disponible
	availablePartition, startPartition, indexPartition := mbr.GetFirstAvailablePartition()
	if availablePartition == nil {
		return errors.New("no hay espacio disponible para la partición extendida")
	}

	for _, partitionName := range mbr.GetPartitionNames() {
		if partitionName == fdisk.name {
			fmt.Println("Ya existe una partición con el nombre especificado.")
			return errors.New("ya existe una partición con el nombre especificado")
		}
	}

	// Crear la partición extendida
	availablePartition.CreatePartition(startPartition, sizeBytes, "E", fdisk.fit, fdisk.name)

	// Asignar la partición en el MBR
	mbr.Mbr_partitions[indexPartition] = *availablePartition

	// Serializar el MBR modificado
	err = mbr.Serialize(fdisk.path)
	if err != nil {
		fmt.Println("Error serializando MBR:", err)
		return fmt.Errorf("error serializando el MBR: %w", err)
	}

	fmt.Println("Partición extendida creada correctamente.")
	return nil
}

// Función para crear una partición lógica
func createLogicalPartition(fdisk *FDISK, sizeBytes int) error {
	var mbr structures.MBR

	// Deserializar el MBR
	err := mbr.Deserialize(fdisk.path)
	if err != nil {
		fmt.Println("Error deserializando MBR:", err)
		return fmt.Errorf("error deserializando el MBR: %w", err)
	}

	// Buscar la partición extendida
	var extendedPartition *structures.Partition
	for i := range mbr.Mbr_partitions {
		if mbr.Mbr_partitions[i].Part_type[0] == 'E' {
			extendedPartition = &mbr.Mbr_partitions[i]
			break
		}
	}

	if extendedPartition == nil {
		return errors.New("no se encontró una partición extendida en el disco")
	}

	// Abrir el archivo del disco
	file, err := os.OpenFile(fdisk.path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	ebrSize := int32(binary.Size(structures.EBR{}))

	// Buscar el último EBR dentro de la partición extendida
	var lastEBR structures.EBR
	var currentEBRPosition int32 = extendedPartition.Part_start
	var lastEBRPosition int32 = -1
	var isFirstEBR bool = true

	//Comienzo con el ciclo para irme moviendo :)
	for {
		// Moverse al offset actual
		file.Seek(int64(currentEBRPosition), 0)

		// Leer el EBR en la posición actual
		err := binary.Read(file, binary.LittleEndian, &lastEBR)
		if err != nil {
			break
		}

		// Si es el primer EBR y está vacío (sin partición lógica)
		if isFirstEBR && lastEBR.Part_size <= 0 {
			break
		}

		isFirstEBR = false
		lastEBRPosition = currentEBRPosition

		// Si no hay más EBRs, salimos del bucle
		if lastEBR.Part_next == -1 {
			break
		}

		// Avanzar al siguiente EBR
		currentEBRPosition = lastEBR.Part_next
	}

	// Calcular la posición para el nuevo EBR
	var newEBRPosition int32
	var newPartitionStart int32

	if lastEBRPosition == -1 {
		// Si no hay EBRs previos, colocamos el nuevo EBR al inicio de la partición extendida
		newEBRPosition = extendedPartition.Part_start
		newPartitionStart = newEBRPosition + ebrSize // La partición comienza después del EBR
	} else {
		// Si hay EBRs previos, calculamos la posición después del último EBR + su partición
		newEBRPosition = lastEBR.Part_start + lastEBR.Part_size
		newPartitionStart = newEBRPosition + ebrSize // La partición comienza después del EBR

		// Verificar que haya espacio suficiente en la partición extendida
		if newPartitionStart+int32(sizeBytes) > extendedPartition.Part_start+extendedPartition.Part_size {
			return errors.New("no hay espacio suficiente en la partición extendida para la nueva partición lógica")
		}
	}

	// Crear el nuevo EBR para la partición lógica
	newEBR := structures.EBR{
		Part_status: [1]byte{'0'},
		Part_fit:    [1]byte{fdisk.fit[0]},
		Part_start:  newPartitionStart, // La partición lógica comienza después del EBR
		Part_size:   int32(sizeBytes),
		Part_next:   -1,
	}

	// Copiar el nombre de la partición al EBR
	copy(newEBR.Part_name[:], fdisk.name)

	// Escribir el nuevo EBR en el archivo del disco
	file.Seek(int64(newEBRPosition), 0)
	err = binary.Write(file, binary.LittleEndian, &newEBR)
	if err != nil {
		fmt.Println("Error escribiendo EBR:", err)
		return err
	}

	// Actualizar el EBR anterior si existe
	if lastEBRPosition != -1 {
		lastEBR.Part_next = newEBRPosition
		file.Seek(int64(lastEBRPosition), 0)
		err = binary.Write(file, binary.LittleEndian, &lastEBR)
		if err != nil {
			fmt.Println("Error actualizando EBR anterior:", err)
			return err
		}
	}

	fmt.Println("Partición lógica creada correctamente.")
	return nil
}
