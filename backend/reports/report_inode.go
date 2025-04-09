package reports

import (
	structures "backend/structures"
	utils "backend/utils"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// ReportInode genera un reporte de un inodo y lo guarda en la ruta especificada
func ReportInode(superblock *structures.SuperBlock, diskPath string, path string) error {
	// Crear las carpetas padre si no existen
	err := utils.CreateParentDirs(path)
	if err != nil {
		return err
	}

	// Obtener el nombre base del archivo sin la extensión
	dotFileName, outputImage := utils.GetFileNames(path)

	// Verificar si el superbloque es válido
	inodeBitmapSize := superblock.S_inodes_count // Total de inodos posibles
	if inodeBitmapSize <= 0 {
		return fmt.Errorf("s_inodes_count (total) es inválido: %d", inodeBitmapSize)
	}

	inodeBitmap := make([]byte, inodeBitmapSize)
	file, err := os.Open(diskPath)
	if err != nil {
		return fmt.Errorf("error al abrir disco para leer bitmap de inodos: %w", err)
	}
	// Asegurarse de buscar desde el inicio del archivo para el offset del bitmap
	_, err = file.Seek(int64(superblock.S_bm_inode_start), 0) // SEEK_SET = 0
	if err != nil {
		file.Close()
		return fmt.Errorf("error al buscar inicio de bitmap de inodos: %w", err)
	}
	bytesRead, err := file.Read(inodeBitmap)
	file.Close() // Cerrar el archivo después de leer
	if err != nil || int32(bytesRead) != inodeBitmapSize {
		return fmt.Errorf("error al leer bitmap de inodos completo (leídos %d, esperados %d): %w", bytesRead, inodeBitmapSize, err)
	}

	// Iniciar el contenido DOT
	dotContent := `digraph G {
		rankdir=LR;
        node [shape=plaintext]
    `

	var lastValidInodeIndex int32 = -1 // Para rastrear el último inodo VÁLIDO

	// Iterar sobre cada inodo
	for i := int32(0); i < superblock.S_inodes_count; i++ {

		if inodeBitmap[i] != '1' {
			continue // Saltar
		}

		currentIndex := i // Guardar el índice actual válido

		inode := &structures.Inode{}
		// Deserializar el inodo
		inodeOffset := int64(superblock.S_inode_start + (currentIndex * superblock.S_inode_size))
		err := inode.Deserialize(diskPath, inodeOffset)
		if err != nil {
			// Si está marcado como usado pero falla la deserialización, es un error del FS
			fmt.Printf("Error deserializando inodo %d (marcado como usado): %v. Generando nodo de error.\n", currentIndex, err)
			dotContent += fmt.Sprintf("\tinode%d [label=\"Error Inodo %d\", shape=box, style=filled, fillcolor=red];\n", currentIndex, currentIndex)
			lastValidInodeIndex = -1 // No conectar desde/hacia nodos de error
			continue                 // Continuar al siguiente índice ----------------------------------------------------------------------------------------------
		}

		// Convertir tiempos a string
		atime := time.Unix(int64(inode.I_atime), 0).Format(time.RFC3339)
		ctime := time.Unix(int64(inode.I_ctime), 0).Format(time.RFC3339)
		mtime := time.Unix(int64(inode.I_mtime), 0).Format(time.RFC3339)

		// Definir el contenido DOT para el inodo actual
		dotContent += fmt.Sprintf(`inode%d [label=<
    <table border="0" cellborder="1" cellspacing="0">
        <tr><td colspan="2" bgcolor="lightblue"><b>REPORTE INODO %d</b></td></tr>
        <tr><td bgcolor="lightgray"><b>i_uid</b></td><td>%d</td></tr>
        <tr><td bgcolor="lightgray"><b>i_gid</b></td><td>%d</td></tr>
        <tr><td bgcolor="lightgray"><b>i_size</b></td><td>%d</td></tr>
        <tr><td bgcolor="lightgray"><b>i_atime</b></td><td>%s</td></tr>
        <tr><td bgcolor="lightgray"><b>i_ctime</b></td><td>%s</td></tr>
        <tr><td bgcolor="lightgray"><b>i_mtime</b></td><td>%s</td></tr>
        <tr><td bgcolor="lightgray"><b>i_type</b></td><td>%c</td></tr>
        <tr><td bgcolor="lightgray"><b>i_perm</b></td><td>%s</td></tr>
        <tr><td colspan="2" bgcolor="lightgreen"><b>BLOQUES DIRECTOS</b></td></tr>
            `, i, i, inode.I_uid, inode.I_gid, inode.I_size, atime, ctime, mtime, rune(inode.I_type[0]), string(inode.I_perm[:]))

		// Agregar los bloques directos a la tabla hasta el índice 11
		for j := 0; j < 15; j++ {
			bgColor := "lightyellow"
			labelPrefix := fmt.Sprintf("Directo [%d]", j)
			if j == 12 {
				bgColor = "orange"
				labelPrefix = "Indirecto [12]"
			}
			if j == 13 {
				bgColor = "red"
				labelPrefix = "Doble Ind. [13]"
			}
			if j == 14 {
				bgColor = "purple"
				labelPrefix = "Triple Ind. [14]"
			}
			dotContent += fmt.Sprintf(`<tr><td bgcolor="%s"><b>%s</b></td><td>%d</td></tr>`, bgColor, labelPrefix, inode.I_block[j])
		}
		dotContent += `</table>>];` // Cerrar label del nodo

		if lastValidInodeIndex != -1 {
			// Conectar el VÁLIDO anterior con el VÁLIDO actual
			dotContent += fmt.Sprintf("\n\tinode%d -> inode%d;", lastValidInodeIndex, currentIndex)
		}
		lastValidInodeIndex = currentIndex // Actualizar el último índice válido encontrado
	}

	dotContent += "\n}" // Cerrar el grafo

	// --- El resto de la función (escribir DOT, ejecutar dot) permanece igual ---
	dotFile, err := os.Create(dotFileName)
	if err != nil {
		return err
	}
	defer dotFile.Close()
	_, err = dotFile.WriteString(dotContent)
	if err != nil {
		return err
	}

	cmd := exec.Command("dot", "-Tpng", dotFileName, "-o", outputImage)
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error ejecutando Graphviz para ReportInode:\n%s\n", string(cmdOutput))
		return fmt.Errorf("error al ejecutar Graphviz: %w", err)
	}

	fmt.Println("Imagen de los inodos generada:", outputImage)
	return nil
}
