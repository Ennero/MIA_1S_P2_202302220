package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

func ParseCat(tokens []string) (string, error) {
	// Verificar que se proporcionó al menos un argumento
	if len(tokens) == 0 {
		return "", fmt.Errorf("faltan parámetros: se requiere al menos un argumento -fileN=[/ruta | \"/ruta\"]")
	}

	// Unir tokens en una sola cadena 
	args := strings.Join(tokens, " ")
	fmt.Printf("Argumentos completos para cat: %s\n", args)

	re := regexp.MustCompile(`-file\d+=([^\s]+)`)

	// Buscar todas las coincidencias y subcoincidencias
	matches := re.FindAllStringSubmatch(args, -1)
	fmt.Printf("Coincidencias con formato -fileN=... encontradas: %v\n", matches)

	// Si no se encontró NINGUNA coincidencia con el formato esperado
	if len(matches) == 0 {
		return "", fmt.Errorf("no se encontraron argumentos con el formato -fileN=[/ruta | \"/ruta\"]")
	}

	var paths []string

	// Extraer los paths de los grupos capturados
	for _, match := range matches {
		if len(match) > 1 {
			value := match[1] // Valor crudo
			fmt.Printf("  Valor extraído de '%s': '%s'\n", match[0], value)

			path := value
			// Verificar si el valor capturado empieza Y termina con comillas dobles
			if len(path) >= 2 && strings.HasPrefix(path, "\"") && strings.HasSuffix(path, "\"") {
				path = strings.Trim(path, "\"")
				fmt.Printf("    Path sin comillas: '%s'\n", path)
			}

			if path == "" {
				return "", fmt.Errorf("el path proporcionado en '%s' no puede ser vacío después de quitar comillas", match[0])
			}

			if !strings.HasPrefix(path, "/") {
				return "", fmt.Errorf("el path '%s' (de '%s') debe ser absoluto (empezar con /)", path, match[0])
			}
			paths = append(paths, path)
		} else {
			fmt.Printf("  Advertencia: Coincidencia inválida encontrada (sin grupo capturado?): %v\n", match)
		}
	}

	// Si hubo matches pero no se extrajeron paths válidos (muy raro con esta lógica)
	if len(paths) == 0 {
		return "", fmt.Errorf("no se pudieron extraer rutas válidas de los argumentos proporcionados")
	}

	fmt.Println("Paths finales a procesar:", paths)

	// Llamar a la lógica del comando con los paths extraídos
	texto, err := commandCat(paths)
	if err != nil {
		return "", err 
	}

	finalOutput := strings.TrimSuffix(texto, "\n")
	return fmt.Sprintf("CAT: Contenido de el/los archivo(s):\n%s", finalOutput), nil
}

func commandCat(paths []string) (string, error) {
	var salidaBuilder strings.Builder

	var partitionID string
	if stores.Auth.IsAuthenticated() {
		partitionID = stores.Auth.GetPartitionID()
	} else {
		return "", errors.New("no se ha iniciado sesión en ninguna partición")
	}

	partitionSuperblock, _, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID) // Usar la función unificada
	if err != nil {
		return "", fmt.Errorf("error al obtener la partición montada '%s': %w", partitionID, err)
	}

	// Validar superbloque por si acaso
	if partitionSuperblock.S_magic != 0xEF53 {
		return "", fmt.Errorf("magia del superbloque inválida (0x%X) para la partición '%s'", partitionSuperblock.S_magic, partitionID)
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return "", fmt.Errorf("tamaño de inodo/bloque inválido en superbloque partición '%s'", partitionID)
	}

	for i, path := range paths {
		fmt.Printf("Procesando path [%d]: %s\n", i+1, path)

		if !strings.HasPrefix(path, "/") {
			return "", fmt.Errorf("error interno: path '%s' no es absoluto", path)
		}

		// Buscar Inodo
		inodeIndex, inode, errFind := structures.FindInodeByPath(partitionSuperblock, partitionPath, path)
		if errFind != nil {
			// Si no se encuentra, DEBE continuar con los otros archivos si hay más
			fmt.Printf("Error: No se encontró el archivo '%s': %v\n", path, errFind)
			salidaBuilder.WriteString(fmt.Sprintf("cat: %s: No such file or directory\n", path))
			continue
		}

		// Verificar si es archivo
		if inode.I_type[0] != '1' {
			fmt.Printf("Error: '%s' (inodo %d) no es un archivo (tipo: %c)\n", path, inodeIndex, inode.I_type[0])
			salidaBuilder.WriteString(fmt.Sprintf("cat: %s: Is a directory\n", path))
			continue
		}

		fmt.Printf("Archivo '%s' encontrado (inodo %d, tamaño %d bytes).\n", path, inodeIndex, inode.I_size)

		// Leer Contenido
		content, errRead := structures.ReadFileContent(partitionSuperblock, partitionPath, inode)
		if errRead != nil {
			fmt.Printf("Error leyendo contenido de '%s': %v\n", path, errRead)
			salidaBuilder.WriteString(fmt.Sprintf("cat: %s: Read error: %v\n", path, errRead))
			continue
		}

		if content == "" {
			fmt.Printf("Archivo '%s' está vacío.\n", path)

		}

		// Añadir contenido al resultado
		fmt.Printf("Contenido leído para '%s': %d bytes\n", path, len(content))
		salidaBuilder.WriteString(content)
	}

	return salidaBuilder.String(), nil // Retornar contenido acumulado
}
