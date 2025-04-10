package commands

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	stores "backend/stores"
	structures "backend/structures"
)

type Edit struct {
	path      string
	contenido string
}

func ParseEdit(tokens []string) (string, error) { 
	cmd := &Edit{}
	processedKeys := make(map[string]bool)

	// Regex para los parámetros esperados
	pathRegex := regexp.MustCompile(`^(?i)-path=(?:"([^"]+)"|([^\s"]+))$`)
	contenidoRegex := regexp.MustCompile(`^(?i)-contenido=(?:"([^"]+)"|([^\s"]+))$`)

	if len(tokens) == 0 {
		return "", errors.New("faltan parámetros: se requiere -path y -contenido")
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
		} else if match = contenidoRegex.FindStringSubmatch(token); match != nil {
			key = "-contenido"
			if match[1] != "" {
				value = match[1]
			} else {
				value = match[2]
			}
			matched = true
		}

		if !matched {
			return "", fmt.Errorf("parámetro inválido o no reconocido: '%s'. Se esperaba -path= o -contenido=", token)
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
		case "-contenido":
			cmd.contenido = value 
		}
	} 

	// Verificar obligatorios
	if !processedKeys["-path"] {
		return "", errors.New("falta el parámetro requerido: -path")
	}
	if !processedKeys["-contenido"] {
		return "", errors.New("falta el parámetro requerido: -contenido")
	}

	// Validar existencia del archivo de contenido
	if _, err := os.Stat(cmd.contenido); os.IsNotExist(err) {
		return "", fmt.Errorf("el archivo especificado en -contenido no existe en el sistema operativo: '%s'", cmd.contenido)
	} else if err != nil {
		return "", fmt.Errorf("error al verificar archivo en -contenido '%s': %w", cmd.contenido, err)
	}

	// Llamar a la lógica del comando
	err := commandEdit(cmd) 
	if err != nil {
		return "", err 
	}

	return fmt.Sprintf("EDIT: Archivo '%s' modificado correctamente.", cmd.path), nil
}

func commandEdit(cmd *Edit) error {
	fmt.Printf("Intentando editar: %s con contenido de %s\n", cmd.path, cmd.contenido)

	// Autenticación y obtener SB/Partición
	if !stores.Auth.IsAuthenticated() {
		return errors.New("comando edit requiere inicio de sesión")
	}
	currentUser, _, partitionID := stores.Auth.GetCurrentUser()
	partitionSuperblock, mountedPartition, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return fmt.Errorf("error obteniendo partición montada '%s': %w", partitionID, err)
	}
	if partitionSuperblock.S_magic != 0xEF53 {
		return errors.New("magia de superbloque inválida")
	}
	if partitionSuperblock.S_inode_size <= 0 || partitionSuperblock.S_block_size <= 0 {
		return errors.New("tamaño de inodo o bloque inválido")
	}

	// Encontrar Inodo del archivo a editar
	fmt.Printf("Buscando inodo para '%s'...\n", cmd.path)
	targetInodeIndex, targetInode, errFind := structures.FindInodeByPath(partitionSuperblock, partitionPath, cmd.path)
	if errFind != nil {
		return fmt.Errorf("error: no se encontró el archivo '%s': %w", cmd.path, errFind)
	}

	// Verificar que es un ARCHIVO
	if targetInode.I_type[0] != '1' {
		return fmt.Errorf("error: '%s' no es un archivo (es tipo %c)", cmd.path, targetInode.I_type[0])
	}

	// Verificar Permisos de Lectura y Escritura
	fmt.Printf("Verificando permisos R/W para usuario '%s' en inodo %d (Perms: %s)...\n", currentUser, targetInodeIndex, string(targetInode.I_perm[:]))
	canReadWrite := false
	if currentUser == "root" {
		canReadWrite = true
	} else {
		ownerPermRead := targetInode.I_perm[0] == 'r' || targetInode.I_perm[0] == 'R' || targetInode.I_perm[0] == '4' || targetInode.I_perm[0] == '5' || targetInode.I_perm[0] == '6' || targetInode.I_perm[0] == '7'
		ownerPermWrite := targetInode.I_perm[1] == 'w' || targetInode.I_perm[1] == 'W' || targetInode.I_perm[1] == '6' || targetInode.I_perm[1] == '7'
		isOwner := true
		if isOwner && ownerPermRead && ownerPermWrite {
			canReadWrite = true
		}
	}
	if !canReadWrite {
		return fmt.Errorf("permiso denegado: el usuario '%s' no tiene permisos de lectura y escritura sobre '%s'", currentUser, cmd.path)
	}
	fmt.Println("Permisos concedidos.")

	// Leer el NUEVO contenido desde el archivo HOST
	fmt.Printf("Leyendo nuevo contenido desde host OS: %s\n", cmd.contenido)
	newContentBytes, errReadHost := os.ReadFile(cmd.contenido)
	if errReadHost != nil {
		return fmt.Errorf("error leyendo archivo de contenido '%s': %w", cmd.contenido, errReadHost)
	}
	newSize := int32(len(newContentBytes))
	fmt.Printf("Nuevo tamaño: %d bytes.\n", newSize)

	// Liberar Bloques ANTIGUOS del inodo
	fmt.Printf("Liberando bloques antiguos del inodo %d...\n", targetInodeIndex)
	// Pasar el puntero al inodo que leímos
	errFree := structures.FreeInodeBlocks(targetInode, partitionSuperblock, partitionPath)
	if errFree != nil {
		fmt.Printf("ADVERTENCIA: Error al liberar bloques antiguos del inodo %d: %v\n", targetInodeIndex, errFree)
		return fmt.Errorf("error liberando bloques antiguos: %w", errFree)
	} else {
		fmt.Println("Bloques antiguos liberados (o no había).")
	}

	// Calcular y Verificar bloques necesarios para NUEVO contenido
	blockSize := partitionSuperblock.S_block_size
	numBlocksNeeded := int32(0)
	if newSize > 0 {
		numBlocksNeeded = (newSize + blockSize - 1) / blockSize
	}
	fmt.Printf("Bloques necesarios para nuevo contenido: %d\n", numBlocksNeeded)
	if numBlocksNeeded > partitionSuperblock.S_free_blocks_count {
		return fmt.Errorf("espacio insuficiente en disco: se necesitan %d bloques, disponibles %d", numBlocksNeeded, partitionSuperblock.S_free_blocks_count)
	}

	// Asignar Nuevos Bloques y Escribir Contenido
	fmt.Printf("Asignando %d bloque(s) y escribiendo nuevo contenido...\n", numBlocksNeeded)
	var newAllocatedBlockIndices [15]int32
	newAllocatedBlockIndices, errAlloc := allocateDataBlocks(newContentBytes, newSize, partitionSuperblock, partitionPath)
	if errAlloc != nil {
		return fmt.Errorf("falló la asignación/escritura de nuevos bloques: %w", errAlloc)
	}

	// Actualizar el Inodo en Memoria
	fmt.Println("Actualizando metadatos del inodo...")
	targetInode.I_size = newSize                   // Nuevo tamaño
	targetInode.I_block = newAllocatedBlockIndices // Nuevos punteros a bloques
	currentTime := float32(time.Now().Unix())
	targetInode.I_mtime = currentTime // Actualizar tiempo de modificación
	targetInode.I_atime = currentTime // Actualizar tiempo de acceso

	// Serializar Inodo Actualizado
	inodeOffset := int64(partitionSuperblock.S_inode_start + targetInodeIndex*partitionSuperblock.S_inode_size)
	fmt.Printf("Serializando inodo %d actualizado en offset %d...\n", targetInodeIndex, inodeOffset)
	err = targetInode.Serialize(partitionPath, inodeOffset)
	if err != nil {
		return fmt.Errorf("error serializando inodo '%s' actualizado: %w", cmd.path, err)
	}

	// Serializar Superbloque 
	fmt.Println("Serializando SuperBlock después de EDIT...")
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		return fmt.Errorf("ADVERTENCIA: error al serializar superbloque después de edit: %w", err)
	}

	fmt.Println("EDIT completado exitosamente.")
	return nil
}
