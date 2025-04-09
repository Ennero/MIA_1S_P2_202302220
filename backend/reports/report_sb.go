package reports

import (
	structures "backend/structures"
	utils "backend/utils"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// ReportSuperBlock genera un reporte del Superbloque con colores diferenciados para información de inodos y bloques
func ReportSuperBlock(superblock *structures.SuperBlock, diskPath string, outputPath string) error {
	// Crear las carpetas padre si no existen
	err := utils.CreateParentDirs(outputPath)
	if err != nil {
		return err
	}

	// Obtener nombres base del archivo DOT y la imagen de salida
	dotFileName, outputImage := utils.GetFileNames(outputPath)

	// Iniciar el contenido DOT con una tabla
	dotContent := fmt.Sprintf(`digraph G {
	node [shape=plaintext]
	tabla [label=<
		<table border="0" cellborder="1" cellspacing="0">
			<tr><td colspan="2" bgcolor="gray"><b> REPORTE SUPERBLOQUE </b></td></tr>
			<tr><td bgcolor="lightblue"><b>s_inodes_count</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightgreen"><b>s_blocks_count</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightblue"><b>s_free_inodes_count</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightgreen"><b>s_free_blocks_count</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightgray"><b>s_mtime</b></td><td>%s</td></tr>
			<tr><td bgcolor="lightgray"><b>s_umtime</b></td><td>%s</td></tr>
			<tr><td bgcolor="lightgray"><b>s_mnt_count</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightgray"><b>s_magic</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightblue"><b>s_inode_size</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightgreen"><b>s_block_size</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightblue"><b>s_first_ino</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightgreen"><b>s_first_blo</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightblue"><b>s_bm_inode_start</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightgreen"><b>s_bm_block_start</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightblue"><b>s_inode_start</b></td><td>%d</td></tr>
			<tr><td bgcolor="lightgreen"><b>s_block_start</b></td><td>%d</td></tr>
		`, superblock.S_inodes_count, superblock.S_blocks_count, superblock.S_free_inodes_count,
		superblock.S_free_blocks_count, time.Unix(int64(superblock.S_mtime), 0), time.Unix(int64(superblock.S_umtime), 0),
		superblock.S_mnt_count, superblock.S_magic, superblock.S_inode_size, superblock.S_block_size, superblock.S_first_ino,
		superblock.S_first_blo, superblock.S_bm_inode_start, superblock.S_bm_block_start, superblock.S_inode_start, superblock.S_block_start)

	// Cerrar la tabla y el contenido DOT
	dotContent += "</table>>] }"

	// Guardar el contenido DOT en un archivo
	file, err := os.Create(dotFileName)
	if err != nil {
		return fmt.Errorf("error al crear el archivo DOT: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString(dotContent)
	if err != nil {
		return fmt.Errorf("error al escribir en el archivo DOT: %v", err)
	}

	// Ejecutar el comando Graphviz para generar la imagen
	cmd := exec.Command("dot", "-Tpng", dotFileName, "-o", outputImage)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error al ejecutar Graphviz: %v", err)
	}

	fmt.Println("Reporte Super Bloque generado:", outputImage)
	return nil
}