package commands

import (
	"errors"
	"fmt"
	"path/filepath" // Para Clean en ParseContent
	"regexp"
	"strings"
	// "time" // No necesario

	stores "backend/stores"
	structures "backend/structures"
)

type CONTENT struct {
	ruta string
}

func ParseContent(tokens []string) (string, error) {
	cmd := &CONTENT{}
	processedKeys := make(map[string]bool)

	rutaRegex := regexp.MustCompile(`^(?i)-ruta=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -ruta=<directorio_interno>")
	}

	// Este comando solo debe tener el parámetro -ruta
	if len(tokens) > 1 {
		return "", fmt.Errorf("demasiados parámetros para content, solo se espera -ruta=<directorio_interno>")
	}
	token := strings.TrimSpace(tokens[0])
	if token == "" {
		return "", errors.New("parámetro vacío proporcionado")
	}

	match := rutaRegex.FindStringSubmatch(token)
	if match != nil {
		key := "-ruta" // La única clave válida ahora
		value := ""
		if match[1] != "" {
			value = match[1]
		} else {
			value = match[2]
		} // Extraer valor

		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado: %s", key)
		}
		processedKeys[key] = true
		if value == "" {
			return "", errors.New("el valor para -ruta no puede estar vacío")
		}

		// Validar que la ruta interna sea absoluta y limpiarla
		if !strings.HasPrefix(value, "/") {
			return "", fmt.Errorf("la ruta interna '%s' debe ser absoluta (empezar con /)", value)
		}
		cleanedPath := filepath.Clean(value)
		// Asegurarse que Clean no elimine el '/' raíz
		if value == "/" && cleanedPath == "." {
			cleanedPath = "/"
		}
		cmd.ruta = cleanedPath // Guardar path limpio

	} else {
		// Si no coincide con -ruta, es un error
		return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -ruta=<directorio_interno>", token)
	}

	// Verificar obligatorio -ruta
	if !processedKeys["-ruta"] {
		return "", errors.New("falta el parámetro requerido: -ruta")
	}

	// Llamar a la lógica del comando
	contentList, err := commandContent(cmd)
	if err != nil {
		return "", err
	}

	// Formatear salida
	if len(contentList) == 0 {
		return fmt.Sprintf("CONTENT: Directorio '%s' está vacío.", cmd.ruta), nil
	}
	return "CONTENT:\n" + strings.Join(contentList, "\n"), nil
}

func commandContent(cmd *CONTENT) ([]string, error) {
	fmt.Printf("Intentando listar contenido de '%s' (en partición activa)\n", cmd.ruta)

	// Verificar Autenticación y obtener info de la partición ACTIVA
	if !stores.Auth.IsAuthenticated() {
		return nil, errors.New("comando content requiere inicio de sesión")
	}
	currentUser, userGIDStr, partitionID := stores.Auth.GetCurrentUser()
	// Obtener SB y path al disco físico de la sesión actual
	partitionSuperblock, _, diskPath, err := stores.GetMountedPartitionSuperblock(partitionID) // diskPath es el path físico correcto
	if err != nil {
		return nil, fmt.Errorf("error obteniendo partición activa '%s': %w", partitionID, err)
	}
	if partitionSuperblock.S_magic != 0xEF53 {
		return nil, errors.New("magia de superbloque inválida")
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return nil, errors.New("tamaño de inodo o bloque inválido")
	}

	// Encontrar Inodo del Directorio (-ruta) usando el diskPath de la sesión
	fmt.Printf("Buscando inodo para directorio interno: %s (en disco %s)\n", cmd.ruta, diskPath)
	targetInodeIndex, targetInode, errFind := structures.FindInodeByPath(partitionSuperblock, diskPath, cmd.ruta)
	if errFind != nil {
		return nil, fmt.Errorf("error: no se encontró el directorio '%s': %w", cmd.ruta, errFind)
	}

	// Verificar que es un Directorio
	if targetInode.I_type[0] != '0' {
		return nil, fmt.Errorf("error: la ruta '%s' no es un directorio (tipo %c)", cmd.ruta, targetInode.I_type[0])
	}
	fmt.Printf("Directorio encontrado (inodo %d)\n", targetInodeIndex)

	// Verificar Permiso de Lectura
	fmt.Printf("Verificando permiso de lectura para usuario '%s' en '%s'...\n", currentUser, cmd.ruta)
	if !checkPermissions(currentUser, userGIDStr, 'r', targetInode, partitionSuperblock, diskPath) { // Asume que checkPermissions existe
		return nil, fmt.Errorf("permiso denegado: usuario '%s' no puede leer '%s'", currentUser, cmd.ruta)
	}
	fmt.Println("Permiso de lectura concedido.")

	// Leer Contenido del Directorio
	fmt.Println("Leyendo entradas del directorio...")
	contentList := []string{}
	for i := 0; i < 12; i++ { 
		blockPtr := targetInode.I_block[i]
		if blockPtr == -1 || blockPtr < 0 || blockPtr >= partitionSuperblock.S_blocks_count {
			continue
		}

		folderBlock := structures.FolderBlock{}
		blockOffset := int64(partitionSuperblock.S_block_start + blockPtr*partitionSuperblock.S_block_size)
		if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
			fmt.Printf("  Advertencia: Error leyendo bloque %d dir %d: %v.\n", blockPtr, targetInodeIndex, err)
			continue
		}
		// Recorrer entradas del bloque
		for j := range folderBlock.B_content {
			entry := folderBlock.B_content[j]
			if entry.B_inodo != -1 { // Si es una entrada válida
				entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")

				// verificar que NO sea "." ni ".." 
				if entryName != "." && entryName != ".." {
					contentList = append(contentList, entryName)
				}
			}
		}
	}
	fmt.Printf("Contenido encontrado: %v\n", contentList)
	return contentList, nil
}
