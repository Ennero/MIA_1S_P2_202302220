package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	utils "backend/utils"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// MKDIR estructura que representa el comando mkdir con sus parámetros
type MKDIR struct {
	path string // Path del directorio
	p    bool   // Opción -p (crea directorios padres si no existen)
}

func ParseMkdir(tokens []string) (string, error) {
	cmd := &MKDIR{} // Crea una nueva instancia de MKDIR

	// Unir tokens en una sola cadena
	args := strings.Join(tokens, " ")

	// Expresión regular para capturar parámetros con y sin comillas
	re := regexp.MustCompile(`-path="([^"]+)"|-path=([^\s]+)|-p`)

	// Encuentra todas las coincidencias de la expresión regular en la cadena de argumentos
	matches := re.FindAllStringSubmatch(args, -1)

	// Verificar que se reconocieron todos los tokens correctamente
	if matches == nil {
		return "", errors.New("sintaxis inválida en los parámetros")
	}

	// Itera sobre cada coincidencia encontrada
	for _, match := range matches {
		if match[1] != "" { // Coincidencia con comillas
			cmd.path = match[1]
		} else if match[2] != "" { // Coincidencia sin comillas
			cmd.path = match[2]
		} else if match[0] == "-p" {
			cmd.p = true
		}
	}

	// Verifica que el parámetro -path haya sido proporcionado
	if cmd.path == "" {
		return "", errors.New("faltan parámetros requeridos: -path")
	}

	// Ejecutar el comando mkdir
	err := commandMkdir(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MKDIR: Directorio %s creado correctamente.", cmd.path), nil
}


func commandMkdir(mkdir *MKDIR) error {
	//Obtengo la parción Motada (como siempre)
	var partitionID string
	if stores.Auth.IsAuthenticated() {
		partitionID = stores.Auth.GetPartitionID()
	} else {
		return errors.New("no se ha iniciado sesión en ninguna partición")
	}
	partitionSuperblock, mountedPartition, partitionPath, err := stores.GetMountedPartitionSuperblock(partitionID)
	if err != nil {
		return fmt.Errorf("error al obtener la partición montada: %w", err)
	}

	//Valido el path
	cleanPath := strings.TrimSuffix(mkdir.path, "/")
	if !strings.HasPrefix(cleanPath, "/") {
		return errors.New("el path debe ser absoluto (empezar con /)")
	}
	if cleanPath == "/" {
		return errors.New("no se puede crear el directorio raíz '/'")
	}
	if cleanPath == "" {
		return errors.New("el path no puede estar vacío")
	}

	//validacion de la p
	if mkdir.p {
		fmt.Println("Creando directorios padres si es necesario...")
		components := strings.Split(cleanPath, "/") // Divide el path en componentes
		currentPathToCheck := "/"

		// Iterar desde el primer componente real (índice 1)
		for i := 1; i < len(components); i++ {
			component := components[i]
			if component == "" {
				continue
			} // Ignorar si hay slashes dobles //

			// Construir el path acumulado
			if currentPathToCheck == "/" {
				currentPathToCheck += component
			} else {
				currentPathToCheck += "/" + component
			}

			fmt.Printf("Verificando/Creando: %s\n", currentPathToCheck)

			// Verificar si existe el directorio actual en la secuencia
			_, inode, errFind := structures.FindInodeByPath(partitionSuperblock, partitionPath, currentPathToCheck)

			if errFind != nil {
				// Si hay un error, se asueme que el directorio no existe

				// TODO: Sería ideal verificar si el error es específicamente "no encontrado"
				// pero como no lo haré así se queda xd

				fmt.Printf("Directorio '%s' no encontrado. Intentando crear...\n", currentPathToCheck)
				parentDirs, destDir := utils.GetParentDirectories(currentPathToCheck)
				errCreate := partitionSuperblock.CreateFolder(partitionPath, parentDirs, destDir)
				if errCreate != nil {
					return fmt.Errorf("error al crear directorio intermedio '%s': %w", currentPathToCheck, errCreate)
				}
				fmt.Printf("Directorio '%s' creado.\n", currentPathToCheck)
			} else {
				// Si existe, verificar que sea un directorio
				if inode.I_type[0] != '0' {
					return fmt.Errorf("error: '%s' existe pero no es un directorio", currentPathToCheck)
				}
				fmt.Printf("Directorio '%s' ya existe.\n", currentPathToCheck)
			}
		}
	} else {
		// si no hay -p, solo verifico el path completo
		parentPath := filepath.Dir(cleanPath) // Obtengo el directorio padre
		fmt.Printf("Verificando existencia del directorio padre: %s\n", parentPath)

		// Verificar si el padre existe y es un directorio
		_, parentInode, errFind := structures.FindInodeByPath(partitionSuperblock, partitionPath, parentPath)
		if errFind != nil {
			// Si hay cualquier error al buscar el padre, asumimos que no existe o es inaccesible
			return fmt.Errorf("error: no se puede crear '%s', el directorio padre '%s' no existe o no se pudo acceder (%w)", mkdir.path, parentPath, errFind)
		}
		if parentInode.I_type[0] != '0' {
			// El padre existe pero no es un directorio
			return fmt.Errorf("error: no se puede crear '%s', '%s' no es un directorio", mkdir.path, parentPath)
		}

		// El padre existe y es un directorio, proceder a crear solo el directorio final
		fmt.Printf("Padre '%s' existe. Creando directorio final '%s'...\n", parentPath, filepath.Base(cleanPath))
		parentDirs, destDir := utils.GetParentDirectories(cleanPath)
		errCreate := partitionSuperblock.CreateFolder(partitionPath, parentDirs, destDir)
		if errCreate != nil {
			// Aquí podría haber un error si el directorio final ya existe.
			// CreateFolder debería idealmente retornar un error específico para "ya existe".
			return fmt.Errorf("error al crear directorio final '%s': %w", mkdir.path, errCreate)
		}
	}
	//Serializo el superbloque después de crear el directorio
	fmt.Println("\nSerializando SuperBlock después de MKDIR...")
	err = partitionSuperblock.Serialize(partitionPath, int64(mountedPartition.Part_start))
	if err != nil {
		// Nota: Si la serialización falla, los cambios podrían perderse al desmontar/reiniciar.
		return fmt.Errorf("error al serializar el superbloque después de mkdir: %w", err)
	}

	partitionSuperblock.PrintInodes(partitionPath)
	partitionSuperblock.PrintBlocks(partitionPath)

	return nil 
}

