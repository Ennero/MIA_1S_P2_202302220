package commands

import (
	"errors"
	"fmt"
	"path/filepath" 
	"regexp"
	"strings"
	stores "backend/stores"
	structures "backend/structures"
)

type CAT struct {
	path string 
	id   string 
}

func ParseCat(tokens []string) (string, error) {
	cmd := &CAT{}
	processedKeys := make(map[string]bool)

	pathRegex := regexp.MustCompile(`^(?i)-path=(?:"([^"]+)"|([^\s"]+))$`)
	idRegex := regexp.MustCompile(`^(?i)-id=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -path=<archivo> y opcionalmente -id=<mount_id>")
	}

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		fmt.Printf("Procesando token CAT: '%s'\n", token)
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
		} else if match = idRegex.FindStringSubmatch(token); match != nil {
			key = "-id"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		}

		if !matched {
			return "", fmt.Errorf("parámetro inválido CAT: '%s'. Se esperaba -path= o -id=", token)
		}
		fmt.Printf("  Match CAT!: key='%s', value='%s'\n", key, value)
		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado CAT: %s", key)
		}
		processedKeys[key] = true
		if value == "" {
			return "", fmt.Errorf("valor vacío para %s", key)
		}

		switch key {
		case "-path":
			if !strings.HasPrefix(value, "/") {
				return "", fmt.Errorf("el path '%s' debe ser absoluto", value)
			}
			cleanedPath := filepath.Clean(value)
			if value == "/" && cleanedPath == "." {
				cleanedPath = "/"
			}
			if cleanedPath == "/" {
				return "", errors.New("no se puede usar cat en el directorio raíz '/'")
			}
			cmd.path = cleanedPath
		case "-id":
			cmd.id = value
		}
	}

	// Verificar obligatorio -path
	if !processedKeys["-path"] {
		return "", errors.New("falta el parámetro requerido: -path")
	}

	fileContent, err := commandCat(cmd)
	if err != nil {
		return "", err
	}

	return fileContent, nil
}

func commandCat(cmd *CAT) (string, error) {

	var targetPartitionID string
	var diskPath string
	var partitionSuperblock *structures.SuperBlock
	var err error

	// Determinar partición
	if cmd.id != "" {
		targetPartitionID = cmd.id
		fmt.Printf("Intentando leer archivo '%s' en partición especificada '%s'\n", cmd.path, targetPartitionID)
		partitionSuperblock, _, diskPath, err = stores.GetMountedPartitionSuperblock(targetPartitionID)
		if err != nil {
			return "", fmt.Errorf("error obteniendo partición '%s': %w", targetPartitionID, err)
		}
	} else {
		fmt.Printf("Intentando leer archivo '%s' en partición activa\n", cmd.path)
		if !stores.Auth.IsAuthenticated() {
			return "", errors.New("cat requiere sesión si no se especifica -id")
		}
		_, _, targetPartitionID = stores.Auth.GetCurrentUser()
		if targetPartitionID == "" {
			return "", errors.New("no hay partición activa y no se especificó -id")
		}
		partitionSuperblock, _, diskPath, err = stores.GetMountedPartitionSuperblock(targetPartitionID)
		if err != nil {
			return "", fmt.Errorf("error obteniendo partición activa '%s': %w", targetPartitionID, err)
		}
	}

	// Validar SB
	if partitionSuperblock.S_magic != 0xEF53 {
		return "", fmt.Errorf("magia inválida en partición '%s'", targetPartitionID)
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return "", fmt.Errorf("tamaño inodo/bloque inválido en '%s'", targetPartitionID)
	}

	// Encontrar Inodo del Archivo
	fmt.Printf("Buscando inodo para archivo: %s (en disco %s)\n", cmd.path, diskPath)
	_, targetInode, errFind := structures.FindInodeByPath(partitionSuperblock, diskPath, cmd.path)
	if errFind != nil {
		return "", fmt.Errorf("error: no se encontró el archivo '%s': %w", cmd.path, errFind)
	}

	// Verificar que es un ARCHIVO
	if targetInode.I_type[0] != '1' {
		return "", fmt.Errorf("error: la ruta '%s' no corresponde a un archivo (es tipo %c)", cmd.path, targetInode.I_type[0])
	}

	// Verificar Permiso de Lectura
	currentUser, userGIDStr, _ := stores.Auth.GetCurrentUser()
	if !stores.Auth.IsAuthenticated() && currentUser != "root" {
		return "", errors.New("se requiere sesión para verificar permisos")
	}
	fmt.Printf("Verificando permiso de lectura para usuario '%s' en '%s'...\n", currentUser, cmd.path)
	if !checkPermissions(currentUser, userGIDStr, 'r', targetInode, partitionSuperblock, diskPath) { // Asume checkPermissions existe
		return "", fmt.Errorf("permiso denegado: usuario '%s' no puede leer '%s'", currentUser, cmd.path)
	}
	fmt.Println("Permiso de lectura concedido.")

	// Leer Contenido del Archivo
	fmt.Printf("Leyendo contenido del archivo (inodo %d)...\n", targetInode.I_uid) // UID no es índice, pero es info útil
	content, errRead := structures.ReadFileContent(partitionSuperblock, diskPath, targetInode)
	if errRead != nil {
		return "", fmt.Errorf("error leyendo contenido de '%s': %w", cmd.path, errRead)
	}

	fmt.Printf("Contenido leído: %d bytes\n", len(content))
	return content, nil 
}
