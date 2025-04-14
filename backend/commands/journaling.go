package commands

import (
	"bytes"           
	"encoding/binary" 
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time" // Para formatear la fecha

	stores "backend/stores"
	structures "backend/structures"
)

type JOURNALING struct {
	ID string // ID de la partición cuyo journal se mostrará
}

func ParseJournaling(tokens []string) (string, error) {
	cmd := &JOURNALING{}
	processedKeys := make(map[string]bool)

	// Regex para -id=<valor>
	idRegex := regexp.MustCompile(`^(?i)-id=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -id=<valor>")
	}

	// Este comando solo debe tener el parámetro -id
	if len(tokens) > 1 {
		return "", fmt.Errorf("demasiados parámetros para journaling, solo se espera -id=<valor>")
	}
	token := strings.TrimSpace(tokens[0])
	if token == "" {
		return "", errors.New("parámetro vacío proporcionado")
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
		if value == "" {
			return "", errors.New("el valor para -id no puede estar vacío")
		}
		cmd.ID = value
	} else {
		return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -id=<valor>", token)
	}

	if !processedKeys["-id"] {
		return "", errors.New("falta el parámetro requerido: -id")
	}

	// Llamar a la lógica del comando, que devolverá el string formateado
	journalOutput, err := commandJournaling(cmd)
	if err != nil {
		return "", err
	}

	// Devolver el resultado formateado directamente
	return journalOutput, nil
}

func commandJournaling(cmd *JOURNALING) (string, error) {
	fmt.Printf("Intentando leer journal para partición ID: %s\n", cmd.ID)

	// Obtener SB, Partición, Path
	sb, _, diskPath, err := stores.GetMountedPartitionSuperblock(cmd.ID) // No necesitamos 'partition' aquí
	if err != nil {
		return "", fmt.Errorf("error obteniendo partición montada '%s': %w", cmd.ID, err)
	}
	if sb.S_magic != 0xEF53 {
		return "", errors.New("magia de superbloque inválida")
	}
	if sb.S_inode_size <= 0 || sb.S_block_size <= 0 {
		return "", errors.New("tamaño de inodo o bloque inválido")
	}

	// VERIFICAR QUE SEA EXT3
	if sb.S_filesystem_type != 3 {
		return "", fmt.Errorf("error: el comando journaling solo aplica a sistemas EXT3 (tipo detectado: %d)", sb.S_filesystem_type)
	}
	fmt.Println("Sistema de archivos EXT3 confirmado.")

	// Encontrar y Leer Inodo del Journal 
	fmt.Println("Buscando inodo del journal (/.journal, inodo 2)...")
	journalInodeIndex := int32(2)
	journalInode := &structures.Inode{}
	journalInodeOffset := int64(sb.S_inode_start + journalInodeIndex*sb.S_inode_size)
	if err := journalInode.Deserialize(diskPath, journalInodeOffset); err != nil {
		return "", fmt.Errorf("error crítico: no se pudo leer el inodo del journal (%d): %w", journalInodeIndex, err)
	}
	if journalInode.I_type[0] != '1' {
		return "", fmt.Errorf("error crítico: el inodo del journal (%d) no es tipo archivo", journalInodeIndex)
	}
	fmt.Printf("Inodo del journal encontrado (Tamaño: %d bytes)\n", journalInode.I_size)

	// Si el journal está vacío en disco, retornar string vacío
	if journalInode.I_size == 0 {
		fmt.Println("El archivo journal está vacío.")
		return "", nil
	}

	// Leer Contenido Completo del Journal
	fmt.Println("Leyendo contenido del archivo journal...")
	journalContentStr, errRead := structures.ReadFileContent(sb, diskPath, journalInode)
	if errRead != nil {
		return "", fmt.Errorf("error leyendo contenido del archivo journal: %w", errRead)
	}
	journalContentBytes := []byte(journalContentStr)
	fmt.Printf("Contenido del journal leído: %d bytes\n", len(journalContentBytes))

	// Deserializar Entradas del Journal
	journalEntries := []structures.Journal{}
	journalEntrySize := int(binary.Size(structures.Journal{}))
	if journalEntrySize <= 0 {
		return "", errors.New("tamaño de struct Journal inválido")
	}

	reader := bytes.NewReader(journalContentBytes)
	entryCount := 0
	for reader.Len() >= journalEntrySize {
		var entry structures.Journal
		err := binary.Read(reader, binary.LittleEndian, &entry)
		if err != nil {
			fmt.Printf("Advertencia: Error deserializando entrada journal #%d: %v. Lectura parcial posible.\n", entryCount+1, err)
			break
		}
		journalEntries = append(journalEntries, entry)
		entryCount++
	}
	fmt.Printf("Deserializadas %d entradas del journal.\n", len(journalEntries))

	if len(journalEntries) == 0 {
		fmt.Println("No se encontraron entradas válidas en el journal.")
		return "", nil // Devolver vacío si no hay entradas válidas
	}

	// Formatear Salida
	var outputBuilder strings.Builder
	dateFormat := "02/01/2006 15:04:05" // Formato DD/MM/YYYY HH:MM:SS

	for i, entry := range journalEntries {
		// Limpiar strings de bytes nulos y espacios extra si es necesario
		op := strings.TrimRight(string(entry.J_content.I_operation[:]), "\x00 ")
		path := strings.TrimRight(string(entry.J_content.I_path[:]), "\x00 ")
		content := strings.TrimRight(string(entry.J_content.I_content[:]), "\x00 ")
		// Escapar comas y punto y comas dentro de los campos si fuera necesario
		dateStr := time.Unix(int64(entry.J_content.I_date), 0).Format(dateFormat)

		// Añadir campos separados por coma
		outputBuilder.WriteString(op)
		outputBuilder.WriteString(",")
		outputBuilder.WriteString(path)
		outputBuilder.WriteString(",")
		outputBuilder.WriteString(content)
		outputBuilder.WriteString(",")
		outputBuilder.WriteString(dateStr)

		// Añadir punto y coma si NO es la última entrada
		if i < len(journalEntries)-1 {
			outputBuilder.WriteString(";")
		}
	}

	fmt.Println("Journal formateado exitosamente.")
	return outputBuilder.String(), nil
}
