package commands

import (
	structures "backend/structures"
	utils "backend/utils"
	"encoding/binary"
	"errors" // Paquete para manejar errores y crear nuevos errores con mensajes personalizados
	"fmt"    // Paquete para formatear cadenas y realizar operaciones de entrada/salida
	"os"
	"regexp"  // Paquete para trabajar con expresiones regulares, útil para encontrar y manipular patrones en cadenas
	"strconv" // Paquete para convertir cadenas a otros tipos de datos, como enteros
	"strings" // Paquete para manipular cadenas, como unir, dividir, y modificar contenido de cadenas
)

type FDISK struct {
	size int    // Tamaño de la partición
	unit string // Unidad de medida del tamaño (K o M)
	fit  string // Tipo de ajuste (BF, FF, WF)
	path string // Ruta del archivo del disco
	typ  string // Tipo de partición (P, E, L)
	name string // Nombre de la partición

	delete string // Opción de eliminar partición
	add    int    // Opción de agregar espacio a la partición
}

func ParseFdisk(tokens []string) (string, error) {
	cmd := &FDISK{}
	processedKeys := make(map[string]bool)

	// Valor entre comillas | Valor sin comillas
	sizeRegex := regexp.MustCompile(`^(?i)-size=(\d+)$`)
	unitRegex := regexp.MustCompile(`^(?i)-unit=(?:"([kKmMBb])"|([kKmMBb]))$`)
	fitRegex := regexp.MustCompile(`^(?i)-fit=(?:"([bfwBF]{2})"|([bfwBF]{2}))$`)
	pathRegex := regexp.MustCompile(`^(?i)-path=(?:"([^"]+)"|([^\s"]+))$`)
	typeRegex := regexp.MustCompile(`^(?i)-type=(?:"([pPeElL])"|([pPeElL]))$`)
	nameRegex := regexp.MustCompile(`^(?i)-name=(?:"([^"]+)"|([^\s"]{1,16}))$`)
	deleteRegex := regexp.MustCompile(`^(?i)-delete=(?:"(fast|full)"|(fast|full))$`)
	addRegex := regexp.MustCompile(`^(?i)-add=(-?\d+)$`)

	fmt.Printf("Tokens FDISK recibidos: %v\n", tokens)

	// --- Procesar Tokens ---
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		fmt.Printf("Procesando token: '%s'\n", token)

		var match []string
		var key string
		var value string
		matched := false

		// Intentar hacer match con cada regex válida
		if match = sizeRegex.FindStringSubmatch(token); match != nil {
			key = "-size"
			value = match[1]
			matched = true
		} else if match = unitRegex.FindStringSubmatch(token); match != nil {
			key = "-unit"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if match = fitRegex.FindStringSubmatch(token); match != nil {
			key = "-fit"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if match = pathRegex.FindStringSubmatch(token); match != nil {
			key = "-path"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if match = typeRegex.FindStringSubmatch(token); match != nil {
			key = "-type"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if match = nameRegex.FindStringSubmatch(token); match != nil {
			key = "-name"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if match = deleteRegex.FindStringSubmatch(token); match != nil { // <-- AÑADIDO
			key = "-delete"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		} else if match = addRegex.FindStringSubmatch(token); match != nil { // <-- AÑADIDO
			key = "-add"
			value = match[1]
			matched = true
		}

		if !matched {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'", token)
		}

		fmt.Printf("  Match!: key='%s', value='%s'\n", key, value)

		// Validar duplicados
		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado: %s", key)
		}
		processedKeys[key] = true

		// Validar valor vacío
		if value == "" {
			return "", fmt.Errorf("el valor para el parámetro %s no puede estar vacío", key)
		}

		// Asignar y Validar Valor Específico
		switch key {
		case "-size":
			size, err := strconv.Atoi(value)
			if err != nil || size <= 0 {
				return "", fmt.Errorf("valor inválido para -size: '%s'. Debe ser un entero positivo", value)
			}
			cmd.size = size
		case "-unit":
			unitUpper := strings.ToUpper(value)
			if unitUpper != "K" && unitUpper != "M" && unitUpper != "B" {
				return "", fmt.Errorf("valor inválido para -unit: '%s'. Debe ser K, M o B", value)
			}
			cmd.unit = unitUpper
		case "-fit":
			fitUpper := strings.ToUpper(value)
			if fitUpper != "BF" && fitUpper != "FF" && fitUpper != "WF" {
				return "", fmt.Errorf("valor inválido para -fit: '%s'. Debe ser BF, FF o WF", value)
			}
			cmd.fit = fitUpper
		case "-path":
			cmd.path = value
		case "-type":
			typeUpper := strings.ToUpper(value)
			if typeUpper != "P" && typeUpper != "E" && typeUpper != "L" {
				return "", fmt.Errorf("valor inválido para -type: '%s'. Debe ser P, E o L", value)
			}
			cmd.typ = typeUpper
		case "-name":
			if len(value) > 16 {
				return "", fmt.Errorf("el valor para -name ('%s') excede los 16 caracteres", value)
			}
			cmd.name = value
		case "-delete": 
			deleteLower := strings.ToLower(value)
			if deleteLower != "fast" && deleteLower != "full" {
				return "", fmt.Errorf("valor inválido para -delete: '%s'. Debe ser 'fast' o 'full'", value)
			}
			cmd.delete = deleteLower
		case "-add": 
			addVal, err := strconv.Atoi(value)
			if err != nil {
				return "", fmt.Errorf("valor inválido para -add: '%s'. Debe ser un número entero (positivo o negativo)", value)
			}
			if addVal == 0 {
				return "", errors.New("el valor para -add no puede ser cero")
			}
			cmd.add = addVal
		}
	} 

	// Validaciones de Combinación y Obligatorios
	operation := "create"
	if cmd.delete != "" {
		operation = "delete"
	}
	if cmd.add != 0 {
		if operation == "delete" {
			return "", errors.New("no se pueden usar -delete y -add simultáneamente")
		}
		operation = "add"
	}
	fmt.Printf("Operación detectada: %s\n", operation)

	// Validar parámetros requeridos
	if !processedKeys["-path"] {
		return "", errors.New("parámetro requerido faltante: -path")
	}
	if !processedKeys["-name"] {
		return "", errors.New("parámetro requerido faltante: -name")
	}

	switch operation {
	case "create":
		if !processedKeys["-size"] || cmd.size <= 0 {
			return "", errors.New("para crear partición, -size debe ser especificado y positivo")
		}
		if processedKeys["-delete"] {
			return "", errors.New("no se puede usar -delete al crear partición")
		}
		if processedKeys["-add"] {
			return "", errors.New("no se puede usar -add al crear partición")
		}
		// Establecer valores por defecto si no se dieron
		if !processedKeys["-unit"] {
			cmd.unit = "K"
		}
		if !processedKeys["-fit"] {
			cmd.fit = "WF"
		}
		if !processedKeys["-type"] {
			cmd.typ = "P"
		}

	case "delete":
		if !processedKeys["-delete"] {
			return "", errors.New("operación delete requiere el parámetro -delete=(fast|full)")
		}
		if processedKeys["-size"] || processedKeys["-unit"] || processedKeys["-fit"] || processedKeys["-type"] || processedKeys["-add"] {
			return "", errors.New("parámetros -size, -unit, -fit, -type, -add no son válidos con -delete")
		}

	case "add":
		if !processedKeys["-add"] || cmd.add == 0 {
			return "", errors.New("operación add requiere el parámetro -add=<valor> (distinto de cero)")
		}
		// -unit es opcional para add, usará default
		if processedKeys["-size"] || processedKeys["-fit"] || processedKeys["-type"] || processedKeys["-delete"] {
			return "", errors.New("parámetros -size, -fit, -type, -delete no son válidos con -add")
		}
		if !processedKeys["-unit"] {
			cmd.unit = "K"
		} // Default a K si no se especifica para add

	}

	resultMsg, err := commandFdisk(cmd, operation)
	if err != nil {
		return "", err
	}
	return resultMsg, nil
}


func commandFdisk(cmd *FDISK, operation string) (string, error) {

	switch operation {
	case "create":
		fmt.Println("Ejecutando operación: CREAR")
		// Convertir tamaño a bytes para creación
		sizeBytes, err := utils.ConvertToBytes(cmd.size, cmd.unit)
		if err != nil {
			return "", fmt.Errorf("error convirtiendo -size a bytes: %w", err)
		}

		// Validar tamaño contra disco
		fileInfo, errStat := os.Stat(cmd.path)
		if errStat != nil {
			return "", fmt.Errorf("error accediendo al disco '%s': %w", cmd.path, errStat)
		}
		if int64(sizeBytes) > fileInfo.Size() {
			return "", fmt.Errorf("tamaño de partición solicitado (%d bytes) excede tamaño del disco (%d bytes)", sizeBytes, fileInfo.Size())
		}
		if sizeBytes <= 0 {
			return "", fmt.Errorf("tamaño calculado de partición debe ser positivo (%d bytes)", sizeBytes)
		}

		// Llamar a la función de creación apropiada según cmd.typ
		switch cmd.typ {
		case "P":
			err = createPrimaryPartition(cmd, sizeBytes)
		case "E":
			err = createExtendedPartition(cmd, sizeBytes)
		case "L":
			err = createLogicalPartition(cmd, sizeBytes)
		default:
			err = fmt.Errorf("tipo de partición desconocido '%s' para creación", cmd.typ)
		}
		if err != nil {
			return "", err
		} 

		// Mensaje de éxito para CREAR
		return fmt.Sprintf("FDISK: Partición '%s' creada exitosamente\n"+
			"-> Path: %s\n"+
			"-> Tamaño: %d%s\n"+
			"-> Tipo: %s\n"+
			"-> Fit: %s",
			cmd.name, cmd.path, cmd.size, cmd.unit, cmd.typ, cmd.fit), nil

	case "delete":
		fmt.Println("Ejecutando operación: DELETE")
		err := deletePartition(cmd) // Llamar a la nueva función de borrado
		if err != nil {
			return "", err
		} 
		// Mensaje de éxito para DELETE
		return fmt.Sprintf("FDISK: Partición '%s' eliminada exitosamente (modo: %s)\n"+
			"-> Path: %s",
			cmd.name, cmd.delete, cmd.path), nil

	case "add":
		fmt.Println("Ejecutando operación: ADD")
		err := addSpaceToPartition(cmd) 
		if err != nil {
			return "", err
		} 
		addDesc := "añadido"
		absAdd := cmd.add
		if cmd.add < 0 {
			addDesc = "quitado"
			absAdd = -absAdd
		}
		return fmt.Sprintf("FDISK: Espacio %s exitosamente a la partición '%s'\n"+
			"-> Path: %s\n"+
			"-> Cantidad: %d%s",
			addDesc, cmd.name, cmd.path, absAdd, cmd.unit), nil

	default:
		return "", fmt.Errorf("operación fdisk desconocida: %s", operation)
	}
}

func createPrimaryPartition(fdisk *FDISK, sizeBytes int) error {
	var mbr structures.MBR
	err := mbr.Deserialize(fdisk.path)
	if err != nil {
		return fmt.Errorf("error deserializando MBR: %w", err)
	}
	mbr.PrintMBR()

	availablePartition, startPartition, indexPartition, errFit := mbr.GetFirstAvailablePartition(int32(sizeBytes), fdisk.fit[0])
	if errFit != nil {
		return errFit
	}


	// Verificar si el nombre ya está tomado (usando la nueva función helper)
	if mbr.IsPartitionNameTaken(fdisk.name, fdisk.path) { // Pasar diskPath para revisar lógicas
		return fmt.Errorf("ya existe una partición con el nombre '%s'", fdisk.name)
	}

	fmt.Println("\nSlot MBR disponible encontrado:", indexPartition)
	fmt.Println("Offset de inicio calculado:", startPartition)
	fmt.Println("Partición a modificar (antes):")
	availablePartition.PrintPartition() // Debug

	// Crear la partición en el slot encontrado
	availablePartition.CreatePartition(int(startPartition), sizeBytes, fdisk.typ, fdisk.fit, fdisk.name)

	fmt.Println("\nPartición creada (modificada):")
	availablePartition.PrintPartition() // Debug

	// Actualizar la entrada en el array MBR 
	mbr.Mbr_partitions[indexPartition] = *availablePartition // Asegurar que se guarda el valor modificado

	fmt.Println("\nParticiones del MBR actualizadas:")
	mbr.PrintPartitions() 

	err = mbr.Serialize(fdisk.path)
	if err != nil {
		return fmt.Errorf("error serializando el MBR: %w", err)
	}
	return nil
}

func createExtendedPartition(fdisk *FDISK, sizeBytes int) error {
	var mbr structures.MBR
	err := mbr.Deserialize(fdisk.path)
	if err != nil {
		return fmt.Errorf("error deserializando MBR: %w", err)
	}

	// Verificar si ya existe una extendida
	if extPart, _ := mbr.GetExtendedPartition(); extPart != nil {
		return errors.New("ya existe una partición extendida en el disco")
	}

	// Verificar si el nombre ya está tomado
	if mbr.IsPartitionNameTaken(fdisk.name, fdisk.path) {
		return fmt.Errorf("ya existe una partición con el nombre '%s'", fdisk.name)
	}

	availablePartition, startPartition, indexPartition, errFit := mbr.GetFirstAvailablePartition(int32(sizeBytes), fdisk.fit[0])
	if errFit != nil {
		return errFit 
	}

	fmt.Printf("\nCreando Partición Extendida '%s' en índice %d, inicio %d, tamaño %d bytes, fit %s\n", fdisk.name, indexPartition, startPartition, sizeBytes, fdisk.fit)
	availablePartition.CreatePartition(int(startPartition), sizeBytes, "E", fdisk.fit, fdisk.name)

	fmt.Println("\nPartición creada (modificada):")
	availablePartition.PrintPartition() 

	mbr.Mbr_partitions[indexPartition] = *availablePartition

	// --- Inicializar primer EBR ---
	firstEBR := structures.EBR{}
	firstEBR.Initialize()                
	firstEBR.Part_start = startPartition // El EBR "vacío" empieza donde la extendida
	// Serializar primer EBR vacío
	file, errOpen := os.OpenFile(fdisk.path, os.O_WRONLY, 0644)
	if errOpen != nil {
		return fmt.Errorf("error abriendo disco para inicializar EBR: %w", errOpen)
	}

	_, errSeek := file.Seek(int64(startPartition), 0)
	if errSeek != nil {
		file.Close()
		return fmt.Errorf("error buscando inicio de extendida para EBR: %w", errSeek)
	}
	errWrite := binary.Write(file, binary.LittleEndian, &firstEBR)
	file.Close() // Cerrar archivo después de escribir EBR
	if errWrite != nil {
		return fmt.Errorf("error escribiendo primer EBR: %w", errWrite)
	}
	fmt.Printf("Primer EBR inicializado en %d\n", startPartition)

	fmt.Println("\nParticiones del MBR actualizadas:")
	mbr.PrintPartitions() 
	err = mbr.Serialize(fdisk.path)
	if err != nil {
		return fmt.Errorf("error serializando el MBR: %w", err)
	}

	fmt.Println("Partición extendida creada correctamente.")
	return nil
}

func createLogicalPartition(fdisk *FDISK, sizeBytes int) error {
	var mbr structures.MBR
	err := mbr.Deserialize(fdisk.path)
	if err != nil {
		return fmt.Errorf("error deserializando MBR: %w", err)
	}

	extendedPartition, extPartIndex := mbr.GetExtendedPartition()
	if extendedPartition == nil {
		return errors.New("no se encontró una partición extendida en el disco")
	}
	_ = extPartIndex

	file, err := os.OpenFile(fdisk.path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error abriendo disco para lógica: %w", err)
	}
	defer file.Close()

	ebrSize := int32(binary.Size(structures.EBR{}))
	if int32(sizeBytes)-ebrSize <= 0 {
		return fmt.Errorf("tamaño solicitado (%d) no es suficiente para EBR + datos (> %d)", sizeBytes, ebrSize)
	}

	var lastEBR structures.EBR
	currentPosition := int64(extendedPartition.Part_start)
	var previousPosition int64 = -1             
	occupiedEnd := extendedPartition.Part_start 
	foundDuplicate := false

	for {
		_, errSeek := file.Seek(currentPosition, 0)
		if errSeek != nil {
			return fmt.Errorf("error buscando EBR en %d: %w", currentPosition, errSeek)
		}

		tempEBR := structures.EBR{}
		errRead := binary.Read(file, binary.LittleEndian, &tempEBR)
		if errRead != nil {
			break
		} 

		// Verificar nombre duplicado
		name := strings.TrimRight(string(tempEBR.Part_name[:]), "\x00")
		if tempEBR.Part_status[0] != 'N' && tempEBR.Part_size > 0 && name == fdisk.name {
			foundDuplicate = true
			break
		}

		lastEBR = tempEBR
		previousPosition = currentPosition
		currentOccupiedEnd := int32(currentPosition) + ebrSize
		if lastEBR.Part_size > 0 {
			currentOccupiedEnd = lastEBR.Part_start + lastEBR.Part_size
		}
		if currentOccupiedEnd > occupiedEnd {
			occupiedEnd = currentOccupiedEnd
		}

		if lastEBR.Part_next == -1 {
			break
		}
		if int64(lastEBR.Part_next) <= currentPosition {
			return errors.New("ciclo detectado en EBRs")
		}
		currentPosition = int64(lastEBR.Part_next)
		if currentPosition < int64(extendedPartition.Part_start) || currentPosition >= int64(extendedPartition.Part_start+extendedPartition.Part_size) {
			return errors.New("puntero EBR Part_next fuera de límites")
		}
	} 

	if foundDuplicate {
		return fmt.Errorf("ya existe una partición lógica con el nombre '%s'", fdisk.name)
	}

	// Calcular dónde poner el nuevo EBR (después del último ocupado)
	newEBRPosition := occupiedEnd
	newPartitionDataStart := newEBRPosition + ebrSize
	newPartitionDataSize := int32(sizeBytes) - ebrSize // Tamaño solo de datos

	// Verificar si cabe
	requiredSpaceEnd := newPartitionDataStart + newPartitionDataSize
	extendedEnd := extendedPartition.Part_start + extendedPartition.Part_size
	if requiredSpaceEnd > extendedEnd {
		return fmt.Errorf("no hay suficiente espacio contiguo al final para partición lógica (req: %d, end: %d)", requiredSpaceEnd, extendedEnd)
	}

	fmt.Printf("\nCreando Partición Lógica '%s' en EBR %d, Datos en %d, tamaño datos %d\n", fdisk.name, newEBRPosition, newPartitionDataStart, newPartitionDataSize)

	// Crear nuevo EBR
	newEBR := structures.EBR{}
	newEBR.Initialize()         // Inicializar a default/vacío
	newEBR.Part_status[0] = '0' // Marcar como activa
	newEBR.Part_fit[0] = fdisk.fit[0]
	newEBR.Part_start = newPartitionDataStart
	newEBR.Part_size = newPartitionDataSize
	copy(newEBR.Part_name[:], fdisk.name)
	newEBR.Part_next = -1 // Es el último por ahora

	// Escribir nuevo EBR
	_, errSeek := file.Seek(int64(newEBRPosition), 0)
	if errSeek != nil {
		return fmt.Errorf("error buscando posición nuevo EBR %d: %w", newEBRPosition, errSeek)
	}
	errWrite := binary.Write(file, binary.LittleEndian, &newEBR)
	if errWrite != nil {
		return fmt.Errorf("error escribiendo nuevo EBR en %d: %w", newEBRPosition, errWrite)
	}

	// Actualizar EBR anterior si existe
	if previousPosition != -1 {
		fmt.Printf("Actualizando Part_next del EBR anterior en %d para que apunte a %d\n", previousPosition, newEBRPosition)
		// Releer EBR anterior
		_, errSeek = file.Seek(previousPosition, 0)
		if errSeek != nil {
			return fmt.Errorf("error buscando EBR anterior %d: %w", previousPosition, errSeek)
		}
		// Usar lastEBR que ya leímos (asumiendo que no cambió el archivo)
		lastEBR.Part_next = int32(newEBRPosition) // Actualizar puntero

		// Reescribir EBR anterior
		_, errSeek = file.Seek(previousPosition, 0)
		if errSeek != nil {
			return fmt.Errorf("error buscando EBR anterior %d para reescribir: %w", previousPosition, errSeek)
		}
		errWrite = binary.Write(file, binary.LittleEndian, &lastEBR)
		if errWrite != nil {
			return fmt.Errorf("error reescribiendo EBR anterior %d actualizado: %w", previousPosition, errWrite)
		}
		fmt.Println("EBR anterior actualizado.")
	} else {
		// Este es el primer EBR lógico, el EBR contenedor inicial debe apuntar a él.
		fmt.Println("Actualizando EBR contenedor inicial para apuntar al primer EBR lógico...")
		var firstEBRContainer structures.EBR
		_, errSeek = file.Seek(int64(extendedPartition.Part_start), 0)
		if errSeek == nil {
			errRead := binary.Read(file, binary.LittleEndian, &firstEBRContainer)
			if errRead != nil {
				err = errRead
			}
		}
		if err != nil {
			return fmt.Errorf("error leyendo EBR contenedor en %d: %w", extendedPartition.Part_start, err)
		}

		firstEBRContainer.Part_next = int32(newEBRPosition) // Actualizar enlace

		_, errSeek = file.Seek(int64(extendedPartition.Part_start), 0)
		if errSeek == nil {
			errWrite = binary.Write(file, binary.LittleEndian, &firstEBRContainer)
			if errWrite != nil {
				err = errWrite
			}
		}
		if err != nil {
			return fmt.Errorf("error escribiendo EBR contenedor actualizado: %w", err)
		}
		fmt.Println("EBR contenedor inicial actualizado.")
	}

	fmt.Println("Partición lógica creada correctamente.")
	return nil
}

// --- NUEVAS FUNCIONES ---

// Lógica para eliminar una partición
func deletePartition(cmd *FDISK) error {
	fmt.Printf("Intentando eliminar partición: Path='%s', Nombre='%s', Modo='%s'\n", cmd.path, cmd.name, cmd.delete)

	// Leer MBR
	var mbr structures.MBR
	if err := mbr.Deserialize(cmd.path); err != nil {
		return fmt.Errorf("error leyendo MBR: %w", err)
	}
	mbr.PrintPartitions()

	// Buscar la partición por nombre (Primaria o Extendida)
	targetPartition := -1
	var partitionInfo structures.Partition
	for i := range mbr.Mbr_partitions {
		p := mbr.Mbr_partitions[i]
		// Comparar nombre, ignorando bytes nulos
		name := strings.TrimRight(string(p.Part_name[:]), "\x00")
		if p.Part_status[0] != 'N' && name == cmd.name {
			targetPartition = i
			partitionInfo = p
			break
		}
	}

	// Si no se encontró en MBR, buscar en Lógicas (dentro de la Extendida)
	var targetEbrPosition int64 = -1
	var prevEbrPosition int64 = -1
	var ebrToDelete structures.EBR
	var extendedPartition *structures.Partition = nil

	if targetPartition == -1 {
		fmt.Println("Partición no encontrada en MBR, buscando Lógicas...")
		extendedPartition, _ = mbr.GetExtendedPartition()
		if extendedPartition == nil {
			return fmt.Errorf("partición '%s' no encontrada ni en MBR ni hay Extendida para buscar Lógicas", cmd.name)
		}

		file, err := os.OpenFile(cmd.path, os.O_RDWR, 0644)
		if err != nil {
			return fmt.Errorf("error abriendo disco para buscar lógica: %w", err)
		}
		defer file.Close()

		currentPos := int64(extendedPartition.Part_start)
		prevPos := int64(-1)

		for {
			_, err = file.Seek(currentPos, 0)
			if err != nil {
				break
			} // Error buscando
			var currentEBR structures.EBR
			err = binary.Read(file, binary.LittleEndian, &currentEBR)
			if err != nil {
				break
			} // Error leyendo

			name := strings.TrimRight(string(currentEBR.Part_name[:]), "\x00")
			if currentEBR.Part_status[0] != 'N' && name == cmd.name {
				// Encontrada
				fmt.Printf("Partición Lógica '%s' encontrada en EBR en offset %d\n", cmd.name, currentPos)
				targetEbrPosition = currentPos
				prevEbrPosition = prevPos // Guardar la posición del EBR anterior
				ebrToDelete = currentEBR
				break
			}

			if currentEBR.Part_next == -1 {
				break
			} // Fin de cadena
			if int64(currentEBR.Part_next) <= currentPos {
				return errors.New("error: ciclo detectado en EBRs")
			}

			prevPos = currentPos
			currentPos = int64(currentEBR.Part_next)
		}
	}

	// Si después de buscar en ambos lados no se encontró
	if targetPartition == -1 && targetEbrPosition == -1 {
		return fmt.Errorf("partición con nombre '%s' no encontrada en el disco", cmd.name)
	}

	// Confirmación del usuario
	fmt.Printf("\n¡ADVERTENCIA! Está a punto de eliminar la partición '%s'.\n", cmd.name)
	if targetPartition != -1 && partitionInfo.Part_type[0] == 'E' {
		fmt.Println("Esto eliminará también TODAS las particiones lógicas contenidas dentro.")
	}



	file, err := os.OpenFile(cmd.path, os.O_RDWR, 0644) // Se necesita RDWR para borrar
	if err != nil {
		return fmt.Errorf("error re-abriendo disco para escritura: %w", err)
	}
	defer file.Close()

	// Lógica de Borrado
	if targetPartition != -1 { // Borrar Primaria o Extendida
		partType := partitionInfo.Part_type[0]
		fmt.Printf("Eliminando Partición %c '%s' en índice %d...\n", partType, cmd.name, targetPartition)

		// Si es Extendida, primero borrar TODO su contenido 
		if partType == 'E' {
			fmt.Println("Eliminando contenido de la Partición Extendida...")
			// Simplificación: Si es 'full', llenar toda la extendida con ceros.
			// Si es 'fast', no hacemos nada extra aquí (solo borraremos la entrada del MBR).
			if cmd.delete == "full" {
				fmt.Printf("Rellenando con ceros el espacio de la Extendida (offset %d, size %d)...\n", partitionInfo.Part_start, partitionInfo.Part_size)
				if err := zeroOutSpace(file, int64(partitionInfo.Part_start), int64(partitionInfo.Part_size)); err != nil {
					fmt.Printf("Advertencia: error al rellenar con ceros la partición extendida: %v\n", err)
				}
			} else {
				fmt.Println("Modo 'fast': No se rellena el espacio de la extendida, solo se elimina la entrada.")
			}
		}

		// Borrar entrada en MBR
		startOffset := partitionInfo.Part_start
		sizeToDelete := partitionInfo.Part_size
		// Poner la entrada a cero/default
		mbr.Mbr_partitions[targetPartition].DeletePartition()
		fmt.Println("Entrada de partición eliminada del MBR.")

		//Si es 'full', borrar contenido físico
		if cmd.delete == "full" && partType != 'E' { 
			fmt.Printf("Rellenando con ceros el espacio de la Partición %c (offset %d, size %d)...\n", partType, startOffset, sizeToDelete)
			if err := zeroOutSpace(file, int64(startOffset), int64(sizeToDelete)); err != nil {
				fmt.Printf("Advertencia: error al rellenar con ceros la partición primaria: %v\n", err)
			}
		}

		// Serializar MBR
		if err := mbr.Serialize(cmd.path); err != nil {
			return fmt.Errorf("error serializando MBR después de eliminar partición: %w", err)
		}

	} else { // Borrar Lógica
		fmt.Printf("Eliminando Partición Lógica '%s' (EBR en %d)...\n", cmd.name, targetEbrPosition)

		// Si es 'full', borrar contenido físico de la LÓGICA
		startOffset := ebrToDelete.Part_start
		sizeToDelete := ebrToDelete.Part_size
		if cmd.delete == "full" && sizeToDelete > 0 {
			fmt.Printf("Rellenando con ceros el espacio de la Partición Lógica (offset %d, size %d)...\n", startOffset, sizeToDelete)
			if err := zeroOutSpace(file, int64(startOffset), int64(sizeToDelete)); err != nil {
				fmt.Printf("Advertencia: error al rellenar con ceros la partición lógica: %v\n", err)
			}
		}

		// Modificar cadena de EBRs para saltar el EBR a eliminar
		// Si hay un EBR anterior, actualizar su Part_next para que apunte al Part_next del EBR eliminado
		if prevEbrPosition != -1 {
			var prevEBR structures.EBR
			_, err = file.Seek(prevEbrPosition, 0)
			if err == nil {
				err = binary.Read(file, binary.LittleEndian, &prevEBR)
			}
			if err != nil {
				return fmt.Errorf("error leyendo EBR anterior en %d para actualizar enlace: %w", prevEbrPosition, err)
			}

			fmt.Printf("Actualizando EBR en %d para que su Part_next (%d) apunte a %d\n", prevEbrPosition, prevEBR.Part_next, ebrToDelete.Part_next)
			prevEBR.Part_next = ebrToDelete.Part_next // Enlazar anterior con siguiente

			_, err = file.Seek(prevEbrPosition, 0) // Volver a escribir el anterior
			if err == nil {
				err = binary.Write(file, binary.LittleEndian, &prevEBR)
			}
			if err != nil {
				return fmt.Errorf("error escribiendo EBR anterior en %d actualizado: %w", prevEbrPosition, err)
			}
			fmt.Println("EBR anterior actualizado.")

		} else {
			// Si no hay EBR anterior, significa que este era el PRIMERO lógico.
			fmt.Println("Eliminando el primer EBR lógico. Actualizando EBR contenedor inicial...")
			var firstEBRContainer structures.EBR
			_, err = file.Seek(int64(extendedPartition.Part_start), 0)
			if err == nil {
				err = binary.Read(file, binary.LittleEndian, &firstEBRContainer)
			}
			if err != nil {
				return fmt.Errorf("error leyendo EBR contenedor en %d: %w", extendedPartition.Part_start, err)
			}

			firstEBRContainer.Part_next = ebrToDelete.Part_next // Actualizar enlace

			_, err = file.Seek(int64(extendedPartition.Part_start), 0) // Volver a escribir contenedor
			if err == nil {
				err = binary.Write(file, binary.LittleEndian, &firstEBRContainer)
			}
			if err != nil {
				return fmt.Errorf("error escribiendo EBR contenedor actualizado: %w", err)
			}
			fmt.Println("EBR contenedor inicial actualizado.")
		}

		// Invalidar/borrar el EBR eliminado en sí mismo
		fmt.Printf("Invalidando EBR en offset %d...\n", targetEbrPosition)
		if err := zeroOutSpace(file, targetEbrPosition, int64(binary.Size(structures.EBR{}))); err != nil {
			fmt.Printf("Advertencia: error al invalidar EBR en %d: %v\n", targetEbrPosition, err)
		}
	}

	fmt.Println("Eliminación completada.")
	return nil
}

// Lógica para añadir/quitar espacio
func addSpaceToPartition(cmd *FDISK) error {
	fmt.Printf("Intentando modificar tamaño: Path='%s', Nombre='%s', Add='%d', Unit='%s'\n", cmd.path, cmd.name, cmd.add, cmd.unit)

	// Leer MBR
	var mbr structures.MBR
	if err := mbr.Deserialize(cmd.path); err != nil {
		return fmt.Errorf("error leyendo MBR: %w", err)
	}
	mbr.PrintPartitions()

	// Buscar la partición por nombre 
	targetPartition := -1
	var partitionInfo structures.Partition
	for i := range mbr.Mbr_partitions {
		p := mbr.Mbr_partitions[i]
		name := strings.TrimRight(string(p.Part_name[:]), "\x00")
		if p.Part_status[0] != 'N' && name == cmd.name {
			if p.Part_type[0] == 'L' { 
				return fmt.Errorf("error: no se puede usar -add en particiones lógicas (nombre '%s')", cmd.name)
			}
			if p.Part_type[0] == 'E' && cmd.add < 0 {
				return errors.New("error: no se puede quitar espacio (-add negativo) de particiones extendidas (requiere eliminar/recrear lógicas)")
			}
			targetPartition = i
			partitionInfo = p
			break
		}
	}

	// Buscar Lógica y retornar error si se encuentra
	if targetPartition == -1 {
		extendedPartition, _ := mbr.GetExtendedPartition()
		if extendedPartition != nil {
			return fmt.Errorf("error: no se puede usar -add en particiones lógicas (nombre '%s')", cmd.name)
		}
		return fmt.Errorf("partición con nombre '%s' no encontrada o es lógica", cmd.name)
	}

	fmt.Printf("Partición '%s' encontrada en índice %d del MBR.\n", cmd.name, targetPartition)

	// Calcular bytes a añadir/quitar
	bytesToAdd, err := utils.ConvertToBytes(cmd.add, cmd.unit)
	if err != nil {
		return fmt.Errorf("error convirtiendo valor de -add a bytes: %w", err)
	} // Ya validamos que no sea 0

	fmt.Printf("Bytes a modificar: %d (Positivo=Añadir, Negativo=Quitar)\n", bytesToAdd)

	// Lógica de Modificación
	currentSize := partitionInfo.Part_size
	currentStart := partitionInfo.Part_start
	currentEnd := currentStart + currentSize

	if bytesToAdd > 0 { // Añadir Espacio 
		fmt.Println("Intentando añadir espacio...")
		// Encontrar espacio libre inmediatamente DESPUÉS
		var spaceAfter int32 = 0
		nextPartitionStart := mbr.Mbr_size // Por defecto, el final del disco

		// Buscar la siguiente partición física que empiece después de esta
		for _, p := range mbr.Mbr_partitions {
			// Considerar solo particiones válidas que empiecen después del final de la actual
			if p.Part_status[0] != 'N' && p.Part_size > 0 && p.Part_start >= currentEnd {
				// Si encontramos una, ver si es la más cercana
				if p.Part_start < nextPartitionStart {
					nextPartitionStart = p.Part_start
				}
			}
		}
		spaceAfter = nextPartitionStart - currentEnd // Espacio libre contiguo después
		fmt.Printf("Espacio libre contiguo encontrado después: %d bytes (hasta %d)\n", spaceAfter, nextPartitionStart)

		if int32(bytesToAdd) > spaceAfter {
			return fmt.Errorf("espacio insuficiente después de la partición '%s'. Se necesitan %d bytes, disponibles %d", cmd.name, bytesToAdd, spaceAfter)
		}

		// Hay espacio, actualizar tamaño
		newSize := currentSize + int32(bytesToAdd)
		mbr.Mbr_partitions[targetPartition].Part_size = newSize
		fmt.Printf("Tamaño de la partición '%s' actualizado a %d bytes.\n", cmd.name, newSize)

	} else { // Quitar Espacio
		fmt.Println("Intentando quitar espacio...")
		bytesToRemove := -int32(bytesToAdd) // Hacerlo positivo
		newSize := currentSize - bytesToRemove

		if newSize <= 0 {
			return fmt.Errorf("error: quitar %d bytes resultaría en tamaño no positivo (%d bytes)", bytesToRemove, newSize)
		}
		// Si es Extendida, verificar que nuevo tamaño no corte lógicas existentes
		if partitionInfo.Part_type[0] == 'E' {
			return errors.New("error: no se puede quitar espacio de particiones extendidas")
		}

		mbr.Mbr_partitions[targetPartition].Part_size = newSize
		fmt.Printf("Tamaño de la partición '%s' actualizado a %d bytes.\n", cmd.name, newSize)

		zeroOutOffset := int64(currentStart + newSize)
		zeroOutSize := int64(bytesToRemove)
		fmt.Printf("Rellenando con ceros espacio eliminado (offset %d, size %d)...\n", zeroOutOffset, zeroOutSize)

	}

	if err := mbr.Serialize(cmd.path); err != nil {
		return fmt.Errorf("error serializando MBR después de modificar partición: %w", err)
	}

	fmt.Println("Modificación de tamaño completada.")
	return nil
}

// Helper para rellenar un área del disco con ceros
func zeroOutSpace(file *os.File, offset int64, size int64) error {
	if size <= 0 {
		return nil
	} // Nada que borrar
	if offset < 0 {
		return errors.New("offset negativo para zeroOutSpace")
	}

	// Usar un buffer de ceros para eficiencia
	chunkSize := 1024 * 4             // Buffer de 4KB
	buffer := make([]byte, chunkSize) // Ya inicializado a ceros

	written := int64(0)
	for written < size {
		// Ir a la posición correcta para este chunk
		currentOffset := offset + written
		_, err := file.Seek(currentOffset, 0)
		if err != nil {
			return fmt.Errorf("error buscando offset %d para borrar: %w", currentOffset, err)
		}

		// Calcular cuánto escribir en esta iteración
		bytesToWrite := int64(chunkSize)
		remaining := size - written
		if bytesToWrite > remaining {
			bytesToWrite = remaining
		}

		// Escribir el chunk (o parte de él)
		n, err := file.Write(buffer[:bytesToWrite])
		if err != nil {
			return fmt.Errorf("error escribiendo ceros en offset %d: %w", currentOffset, err)
		}
		if int64(n) != bytesToWrite {
			return fmt.Errorf("escritura incompleta de ceros en offset %d (%d vs %d)", currentOffset, n, bytesToWrite)
		}

		written += bytesToWrite
	}
	return nil
}
