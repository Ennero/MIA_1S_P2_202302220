package commands

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	stores "backend/stores"
	structures "backend/structures"
)

type FIND struct {
	path string 
	name string 
}

func ParseFind(tokens []string) (string, error) {
	cmd := &FIND{}
	processedKeys := make(map[string]bool)

	pathRegex := regexp.MustCompile(`^(?i)-path=(?:"([^"]+)"|([^\s"]+))$`)
	nameRegex := regexp.MustCompile(`^(?i)-name=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -path y -name")
	}

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

		if match = pathRegex.FindStringSubmatch(token); match != nil {
			key = "-path"
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
		}

		if !matched {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -path= o -name=", token)
		}
		fmt.Printf("  Match!: key='%s', value='%s'\n", key, value)

		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado: %s", key)
		}
		processedKeys[key] = true
		if value == "" {
			return "", fmt.Errorf("el valor para %s no puede estar vacío", key)
		}

		switch key {
		case "-path":
			if !strings.HasPrefix(value, "/") {
				return "", fmt.Errorf("el path '%s' debe ser absoluto", value)
			}
			cmd.path = value
		case "-name":
			cmd.name = value
		}
	}

	// Verificar obligatorios
	if !processedKeys["-path"] {
		return "", errors.New("falta el parámetro requerido: -path")
	}
	if !processedKeys["-name"] {
		return "", errors.New("falta el parámetro requerido: -name")
	}

	// Llamar a la lógica del comando
	resultPaths, err := commandFind(cmd) 
	if err != nil {
		return "", err
	}

	// Formatear salida
	if len(resultPaths) == 0 {
		return fmt.Sprintf("FIND: No se encontraron coincidencias para '%s' en '%s'.", cmd.name, cmd.path), nil
	}

	// Construir el string de salida con un path por línea
	var output strings.Builder
	output.WriteString(fmt.Sprintf("FIND: Coincidencias para '%s' en '%s':\n", cmd.name, cmd.path))
	for _, p := range resultPaths {
		output.WriteString(p)
		output.WriteString("\n")
	}

	return strings.TrimSuffix(output.String(), "\n"), nil // Quitar último salto de línea
}

func commandFind(cmd *FIND) ([]string, error) { // Devuelve slice de paths encontrados
	fmt.Printf("Iniciando búsqueda: path='%s', name_pattern='%s'\n", cmd.path, cmd.name)

	// Autenticación y obtener SB/Partición
	if !stores.Auth.IsAuthenticated() {
		return nil, errors.New("comando find requiere inicio de sesión")
	}
	currentUser, userGIDStr, partitionID := stores.Auth.GetCurrentUser()
	partitionSuperblock, _, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return nil, fmt.Errorf("error obteniendo partición montada '%s': %w", partitionID, err)
	}
	if partitionSuperblock.S_magic != 0xEF53 {
		return nil, errors.New("magia de superbloque inválida")
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return nil, errors.New("tamaño de inodo o bloque inválido")
	}

	// Validar Path de Inicio
	fmt.Printf("Validando path de inicio: %s\n", cmd.path)
	startInodeIndex, startInode, errFindStart := structures.FindInodeByPath(partitionSuperblock, partitionPath, cmd.path)
	if errFindStart != nil {
		return nil, fmt.Errorf("error: no se encontró el path de inicio '%s': %w", cmd.path, errFindStart)
	}
	if startInode.I_type[0] != '0' {
		return nil, fmt.Errorf("error: el path de inicio '%s' no es un directorio", cmd.path)
	}
	// Verificar permiso de LECTURA en el directorio inicial
	if !checkPermissions(currentUser, userGIDStr, 'r', startInode, partitionSuperblock, partitionPath) {
		return nil, fmt.Errorf("permiso denegado: lectura sobre directorio de inicio '%s'", cmd.path)
	}
	fmt.Println("Path de inicio validado.")

	// Convertir patrón de nombre a Regex
	regexPattern := convertWildcardToRegex(cmd.name)
	fmt.Printf("Patrón de nombre '%s' convertido a Regex: '^%s$'\n", cmd.name, regexPattern)
	nameMatcher, errCompile := regexp.Compile("^" + regexPattern + "$")
	if errCompile != nil {
		return nil, fmt.Errorf("error interno al compilar regex para el patrón '%s': %w", cmd.name, errCompile)
	}

	// Iniciar Búsqueda Recursiva
	var results []string // Slice para almacenar los paths completos encontrados
	fmt.Println("Iniciando búsqueda recursiva...")

	errFind := recursiveFind(cmd.path, startInodeIndex, nameMatcher, partitionSuperblock, partitionPath, currentUser, userGIDStr, &results)
	if errFind != nil {
		return nil, fmt.Errorf("error durante la búsqueda recursiva: %w", errFind)
	}

	fmt.Printf("Búsqueda completada. Encontrados %d resultados.\n", len(results))
	return results, nil // Devolver slice de paths encontrados
}

// Busca recursivamente archivos/carpetas que coincidan con nameMatcher.
func recursiveFind(
	currentDirPath string, // Path completo del directorio actual
	currentInodeIndex int32,
	nameMatcher *regexp.Regexp, // Regex compilada para el nombre
	sb *structures.SuperBlock,
	diskPath string,
	currentUser string,
	userGIDStr string,
	results *[]string, // Puntero al slice de resultados
) error {
	fmt.Printf("--> recursiveFind: Explorando inodo %d ('%s')\n", currentInodeIndex, currentDirPath)

	// Leer Inodo Actual
	currentInode := &structures.Inode{}
	currentInodeOffset := int64(sb.S_inode_start + currentInodeIndex*sb.S_inode_size)
	if err := currentInode.Deserialize(diskPath, currentInodeOffset); err != nil {
		// No se puede leer este inodo, reportar y no continuar por esta rama
		fmt.Printf("    Error leyendo inodo %d en '%s': %v. Saltando.\n", currentInodeIndex, currentDirPath, err)
		return nil // No es fatal para la búsqueda general, solo esta rama
	}

	// Asegurarse que es un directorio (la primera llamada ya lo valida, pero las recursivas no)
	if currentInode.I_type[0] != '0' {
		fmt.Printf("    Advertencia: Inodo %d ('%s') no es directorio, no se puede buscar dentro.\n", currentInodeIndex, currentDirPath)
		return nil
	}

	// Verificar Permiso de LECTURA en este directorio
	if !checkPermissions(currentUser, userGIDStr, 'r', currentInode, sb, diskPath) {
		fmt.Printf("    Permiso de lectura denegado en '%s' (inodo %d). Omitiendo.\n", currentDirPath, currentInodeIndex)
		return nil // No podemos buscar aquí, pero no detener la búsqueda global
	}
	fmt.Printf("    Permiso de lectura concedido para '%s'.\n", currentDirPath)

	// Iterar Bloques del Directorio
	for i := 0; i < 12; i++ { // Solo directos
		blockPtr := currentInode.I_block[i]
		if blockPtr == -1 || blockPtr < 0 || blockPtr >= sb.S_blocks_count {
			continue
		}

		folderBlock := structures.FolderBlock{}
		blockOffset := int64(sb.S_block_start + blockPtr*sb.S_block_size)
		if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
			fmt.Printf("      Advertencia: Error leyendo bloque %d del dir %d: %v. Saltando bloque.\n", blockPtr, currentInodeIndex, err)
			continue
		}

		// Iterar Entradas del Bloque
		for j := range folderBlock.B_content {
			entry := folderBlock.B_content[j]
			if entry.B_inodo == -1 {
				continue
			} // Slot libre

			entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
			if entryName == "." || entryName == ".." {
				continue
			} 

			childInodeIndex := entry.B_inodo

			// Verificar si el NOMBRE de la entrada coincide con el patrón
			fmt.Printf("        Comparando '%s' con patrón '%s'... ", entryName, nameMatcher.String())
			if nameMatcher.MatchString(entryName) {
				fmt.Println("¡Match!")
				// Construir path completo del item encontrado
				var fullPath string
				if currentDirPath == "/" {
					fullPath = "/" + entryName
				} else {
					fullPath = currentDirPath + "/" + entryName
				}

				// Añadir al slice de resultados (usando puntero)
				*results = append(*results, fullPath)
				fmt.Printf("          Añadido a resultados: %s\n", fullPath)
			} else {
				fmt.Println("No match.")
			}

			// Si la entrada es un subdirectorio, llamar recursivamente
			// Leer el inodo hijo para saber su tipo
			childInode := &structures.Inode{}
			// Validar índice antes de usar
			if childInodeIndex >= 0 && childInodeIndex < sb.S_inodes_count {
				childInodeOffset := int64(sb.S_inode_start + childInodeIndex*sb.S_inode_size)
				if err := childInode.Deserialize(diskPath, childInodeOffset); err == nil {
					// Solo continuar si pudimos leer el inodo hijo
					if childInode.I_type[0] == '0' { // Es un directorio
						// Construir path completo para la llamada recursiva
						var childFullPath string
						if currentDirPath == "/" {
							childFullPath = "/" + entryName
						} else {
							childFullPath = currentDirPath + "/" + entryName
						}
						// Llamada recursiva
						fmt.Printf("        Entrando recursivamente en '%s' (inodo %d)...\n", childFullPath, childInodeIndex)
						errRec := recursiveFind(childFullPath, childInodeIndex, nameMatcher, sb, diskPath, currentUser, userGIDStr, results)
						if errRec != nil {
							fmt.Printf("        Error retornando de recursión en '%s': %v\n", childFullPath, errRec)
							return errRec
						}
					}
				} else {
					fmt.Printf("        Advertencia: No se pudo leer inodo hijo %d (nombre '%s') para recursión: %v\n", childInodeIndex, entryName, err)
				}
			} else {
				fmt.Printf("        Advertencia: Índice de inodo inválido %d encontrado para entrada '%s'.\n", childInodeIndex, entryName)
			}
		} 
	} 

	fmt.Printf("<-- recursiveFind: Saliendo de inodo %d ('%s')\n", currentInodeIndex, currentDirPath)
	return nil 
}

func convertWildcardToRegex(pattern string) string {
	var result strings.Builder
	for _, char := range pattern {
		switch char {
		case '*':
			result.WriteString(".+") // Uno o más caracteres CUALQUIERA
		case '?':
			result.WriteRune('.') // Un caracter CUALQUIERA
		case '.', '\\', '+', '(', ')', '[', ']', '{', '}', '^', '$', '|': // Caracteres especiales de Regex
			result.WriteRune('\\') // Escaparlos con backslash
			result.WriteRune(char)
		default:
			result.WriteRune(char) // Añadir caracter literal
		}
	}
	return result.String()
}
