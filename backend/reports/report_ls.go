package reports

import (
	structures "backend/structures"
	utils "backend/utils"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// --- Implementación del Reporte LS ---

func ReportLS(sb *structures.SuperBlock, diskPath string, outputPath string, targetPath string) error {
	fmt.Printf("Generando reporte LS para: %s en disco: %s, salida: %s\n", targetPath, diskPath, outputPath)

	// 0. Crear directorios de salida y obtener nombres de archivo
	err := utils.CreateParentDirs(outputPath)
	if err != nil {
		return fmt.Errorf("error creando directorios padre para reporte LS: %v", err)
	}
	dotFileName, outputImage := utils.GetFileNames(outputPath)

	// 1. Encontrar el inodo del directorio objetivo (targetPath)
	targetInodeNum, targetInode, err := structures.FindInodeByPath(sb, diskPath, targetPath)
	if err != nil {
		return fmt.Errorf("error al buscar el path '%s' para reporte LS: %v", targetPath, err)
	}

	// 2. Verificar que sea un directorio
	if targetInode.I_type[0] != '0' {
		return fmt.Errorf("el path '%s' no es un directorio, no se puede generar reporte LS", targetPath)
	}

	// 3. Obtener los mapas de UID/GID a Nombres desde users.txt
	uidMap, gidMap, err := getUserGroupNameMaps(sb, diskPath)
	if err != nil {
		// Podrías decidir continuar y mostrar IDs numéricos, o fallar.
		// Por ahora, fallaremos si hay un error irrecuperable en getUserGroupNameMaps.
		return fmt.Errorf("error al obtener mapeo de usuarios/grupos: %v", err)
		// Opcional: Continuar mostrando IDs
		// fmt.Printf("Advertencia: %v. Mostrando IDs numéricos.\n", err)
		// uidMap = make(map[int32]string)
		// gidMap = make(map[int32]string)
	}

	// 4. Preparar el contenido DOT para Graphviz
	dotContent := "digraph G {\n"
	dotContent += "\tnode [shape=none];\n"
	dotContent += "\tgraph [splines=false];\n"
	dotContent += "\tls_report [label=<\n" // Nombre del nodo de la tabla
	dotContent += "\t\t<TABLE BORDER=\"1\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"5\">\n"

	// 4.1. Fila de Encabezado
	dotContent += "\t\t<TR>\n"
	dotContent += "\t\t\t<TD BGCOLOR=\"lightgrey\"><B>Permisos</B></TD>\n"
	dotContent += "\t\t\t<TD BGCOLOR=\"lightgrey\"><B>Owner</B></TD>\n"
	dotContent += "\t\t\t<TD BGCOLOR=\"lightgrey\"><B>Grupo</B></TD>\n"
	dotContent += "\t\t\t<TD BGCOLOR=\"lightgrey\"><B>Size (Bytes)</B></TD>\n"
	dotContent += "\t\t\t<TD BGCOLOR=\"lightgrey\"><B>Fecha Mod.</B></TD>\n"
	dotContent += "\t\t\t<TD BGCOLOR=\"lightgrey\"><B>Hora Mod.</B></TD>\n"
	dotContent += "\t\t\t<TD BGCOLOR=\"lightgrey\"><B>Tipo</B></TD>\n"
	dotContent += "\t\t\t<TD BGCOLOR=\"lightgrey\"><B>Name</B></TD>\n"
	dotContent += "\t\t</TR>\n"

	// 5. Iterar sobre los bloques del directorio objetivo
	inodeBlock := &structures.FolderBlock{} // Para reutilizar la estructura
	entryInode := &structures.Inode{}       // Para reutilizar la estructura

	for _, blockPtr := range targetInode.I_block {
		if blockPtr == -1 {
			continue // Puntero no usado
		}
		if blockPtr < 0 || blockPtr >= sb.S_blocks_count {
			fmt.Printf("Advertencia: Puntero de bloque inválido (%d) encontrado en inodo %d.\n", blockPtr, targetInodeNum)
			continue
		}

		blockOffset := int64(sb.S_block_start) + int64(blockPtr)*int64(sb.S_block_size)
		err := inodeBlock.Deserialize(diskPath, blockOffset)
		if err != nil {
			fmt.Printf("Advertencia: Error al leer bloque de directorio %d: %v. Saltando bloque.\n", blockPtr, err)
			continue
		}

		// 6. Iterar sobre las entradas (FolderContent) dentro de cada bloque
		for _, entry := range inodeBlock.B_content {
			if entry.B_inodo == -1 {
				continue // Entrada no usada
			}
			if entry.B_inodo < 0 || entry.B_inodo >= sb.S_inodes_count {
				fmt.Printf("Advertencia: Puntero de inodo inválido (%d) encontrado en bloque %d.\n", entry.B_inodo, blockPtr)
				continue
			}

			entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
			if entryName == "." || entryName == ".." {
				continue // Omitir entradas '.' y '..' según el formato ls típico
			}

			// 7. Obtener el inodo de la entrada
			entryInodeOffset := int64(sb.S_inode_start) + int64(entry.B_inodo)*int64(sb.S_inode_size)
			err := entryInode.Deserialize(diskPath, entryInodeOffset)
			if err != nil {
				fmt.Printf("Advertencia: Error al leer inodo %d para '%s': %v. Saltando entrada.\n", entry.B_inodo, entryName, err)
				continue
			}

			// 8. Extraer y formatear datos para la fila de la tabla
			permisos := formatPermissions(entryInode.I_perm, entryInode.I_type[0])
			ownerName, ok := uidMap[entryInode.I_uid]
			if !ok {
				ownerName = fmt.Sprintf("%d", entryInode.I_uid) // Mostrar ID si no se encuentra el nombre
			}
			groupName, ok := gidMap[entryInode.I_gid]
			if !ok {
				groupName = fmt.Sprintf("%d", entryInode.I_gid) // Mostrar ID si no se encuentra el nombre
			}
			size := entryInode.I_size
			modTime := time.Unix(int64(entryInode.I_mtime), 0)
			fechaMod := modTime.Format("02/01/2006") // Formato DD/MM/YYYY
			horaMod := modTime.Format("15:04")       // Formato HH:MM (24h)
			tipo := "Archivo"
			if entryInode.I_type[0] == '0' {
				tipo = "Carpeta"
			}

			// 9. Añadir la fila a dotContent
			dotContent += "\t\t<TR>\n"
			dotContent += fmt.Sprintf("\t\t\t<TD>%s</TD>\n", permisos)
			dotContent += fmt.Sprintf("\t\t\t<TD>%s</TD>\n", ownerName)
			dotContent += fmt.Sprintf("\t\t\t<TD>%s</TD>\n", groupName)
			dotContent += fmt.Sprintf("\t\t\t<TD ALIGN=\"RIGHT\">%d</TD>\n", size) // Alinear tamaño a la derecha
			dotContent += fmt.Sprintf("\t\t\t<TD>%s</TD>\n", fechaMod)
			dotContent += fmt.Sprintf("\t\t\t<TD>%s</TD>\n", horaMod)
			dotContent += fmt.Sprintf("\t\t\t<TD>%s</TD>\n", tipo)
			dotContent += fmt.Sprintf("\t\t\t<TD>%s</TD>\n", entryName)
			dotContent += "\t\t</TR>\n"
		}
	}

	// 10. Cerrar la tabla y el grafo DOT
	dotContent += "\t\t</TABLE>\n"
	dotContent += "\t>];\n" // Cierra el label del nodo de la tabla
	dotContent += "}\n"

	// 11. Guardar el archivo .dot
	dotFile, err := os.Create(dotFileName)
	if err != nil {
		return fmt.Errorf("error al crear el archivo DOT para reporte LS: %v", err)
	}
	defer dotFile.Close()

	_, err = dotFile.WriteString(dotContent)
	if err != nil {
		return fmt.Errorf("error al escribir en el archivo DOT para reporte LS: %v", err)
	}
	fmt.Println("Archivo DOT generado:", dotFileName)

	// 12. Ejecutar Graphviz para generar la imagen
	// Asegúrate de que 'dot' esté en el PATH del sistema
	cmd := exec.Command("dot", "-Tpng", dotFileName, "-o", outputImage)
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Salida de Graphviz:\n%s\n", string(cmdOutput))
		return fmt.Errorf("error al ejecutar Graphviz para reporte LS: %v", err)

	}

	fmt.Println("Reporte LS generado exitosamente:", outputImage)
	return nil
}

// --- Funciones Auxiliares (Podrían ir en report_utils.go) ---

// formatPermissions convierte los permisos numéricos y el tipo de archivo a formato string (ej: -rwxrwxrwx)
func formatPermissions(perm [3]byte, fileType byte) string {
	permStr := ""
	if fileType == '0' {
		permStr += "d" // Directorio
	} else {
		permStr += "-" // Archivo
	}

	for _, p := range perm {
		digit, err := strconv.Atoi(string(p))
		if err != nil {
			// Error improbable si los permisos son siempre '0'-'7'
			permStr += "---"
			continue
		}
		// Bit 4: Lectura (r)
		if (digit & 4) != 0 {
			permStr += "r"
		} else {
			permStr += "-"
		}
		// Bit 2: Escritura (w)
		if (digit & 2) != 0 {
			permStr += "w"
		} else {
			permStr += "-"
		}
		// Bit 1: Ejecución (x)
		if (digit & 1) != 0 {
			permStr += "x"
		} else {
			permStr += "-"
		}
	}
	return permStr
}

// parseUsersTxt analiza el contenido de users.txt y devuelve mapas de UID/GID a nombres.
func parseUsersTxt(content string) (map[int32]string, map[int32]string, error) {
	uidToName := make(map[int32]string)
	gidToName := make(map[int32]string)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			fmt.Printf("Advertencia: Línea mal formada en users.txt: %s\n", line)
			continue
		}

		id64, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 32)
		if err != nil {
			fmt.Printf("Advertencia: ID inválido en users.txt: %s\n", parts[0])
			continue
		}
		id := int32(id64)
		userType := strings.TrimSpace(parts[1])
		name := strings.TrimSpace(parts[2])

		if userType == "U" { // Usuario
			if len(parts) < 4 { // Necesita al menos ID, U, username, password
				fmt.Printf("Advertencia: Línea de usuario incompleta en users.txt: %s\n", line)
				continue
			}
			uidToName[id] = name
		} else if userType == "G" { // Grupo
			gidToName[id] = name
		}
	}

	// Asegurarse de que root exista si no está en el archivo (aunque debería)
	if _, ok := uidToName[1]; !ok {
		uidToName[1] = "root"
	}
	if _, ok := gidToName[1]; !ok {
		gidToName[1] = "root"
	}

	return uidToName, gidToName, nil
}

// getUserGroupName obtiene el nombre de usuario o grupo desde users.txt
// Necesita leer y parsear users.txt
func getUserGroupNameMaps(sb *structures.SuperBlock, diskPath string) (map[int32]string, map[int32]string, error) {
	// Asumimos que users.txt está en el inodo 1 (según tu CreateUsersFile)
	usersInode := &structures.Inode{}
	usersInodeOffset := int64(sb.S_inode_start) + 1*int64(sb.S_inode_size) // Offset del inodo 1
	err := usersInode.Deserialize(diskPath, usersInodeOffset)
	if err != nil {
		return nil, nil, fmt.Errorf("error al leer inodo de users.txt: %v", err)
	}

	if usersInode.I_type[0] != '1' {
		return nil, nil, errors.New("el inodo 1 no es un archivo (se esperaba users.txt)")
	}

	usersContent, err := structures.ReadFileContent(sb, diskPath, usersInode)
	if err != nil {
		// Intenta devolver mapas vacíos si no se puede leer users.txt
		fmt.Printf("Advertencia: No se pudo leer el contenido de users.txt: %v. Se usarán IDs numéricos.\n", err)
		return make(map[int32]string), make(map[int32]string), nil // Devuelve mapas vacíos en lugar de error fatal
		// return nil, nil, fmt.Errorf("error al leer contenido de users.txt: %v", err)
	}

	return parseUsersTxt(usersContent)
}
