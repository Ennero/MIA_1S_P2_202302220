package reports

import (
	structures "backend/structures"
	utils "backend/utils"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ReportMBR genera un reporte del MBR con particiones primarias, extendidas y lógicas
func ReportMBR(mbr *structures.MBR, diskPath string, outputPath string) error {
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
            <tr><td colspan="2" bgcolor="gray"><b> REPORTE MBR </b></td></tr>
            <tr><td bgcolor="lightgray"><b>mbr_tamano</b></td><td>%d</td></tr>
            <tr><td bgcolor="lightgray"><b>mbr_fecha_creacion</b></td><td>%s</td></tr>
            <tr><td bgcolor="lightgray"><b>mbr_disk_signature</b></td><td>%d</td></tr>
        `, mbr.Mbr_size, time.Unix(int64(mbr.Mbr_creation_date), 0), mbr.Mbr_disk_signature)

	// Iterar sobre las particiones
	for i, part := range mbr.Mbr_partitions {
		// Ignorar particiones vacías
		if part.Part_size == -1 {
			continue
		}

		// Convertir atributos a string sin caracteres nulos
		partName := strings.TrimRight(string(part.Part_name[:]), "\x00")
		partStatus := rune(part.Part_status[0])
		partType := rune(part.Part_type[0])
		partFit := rune(part.Part_fit[0])

		// Determinar color según el tipo de partición
		var color string
		switch partType {
		case 'P':
			color = "lightblue" // Primaria
		case 'E':
			color = "orange" // Extendida
		default:
			color = "white"
		}

		// Agregar la partición a la tabla
		dotContent += fmt.Sprintf(`
        <tr><td colspan="2" bgcolor="%s"><b> PARTICIÓN %d </b></td></tr>
        <tr><td bgcolor="lightgray"><b>part_status</b></td><td>%c</td></tr>
        <tr><td bgcolor="lightgray"><b>part_type</b></td><td>%c</td></tr>
        <tr><td bgcolor="lightgray"><b>part_fit</b></td><td>%c</td></tr>
        <tr><td bgcolor="lightgray"><b>part_start</b></td><td>%d</td></tr>
        <tr><td bgcolor="lightgray"><b>part_size</b></td><td>%d</td></tr>
        <tr><td bgcolor="lightgray"><b>part_name</b></td><td>%s</td></tr>
    `, color, i+1, partStatus, partType, partFit, part.Part_start, part.Part_size, partName)

		// Si es una partición extendida, buscar sus particiones lógicas
		if partType == 'E' {
			dotContent += `<tr><td colspan="2" bgcolor="lightgreen"><b> Particiones Lógicas </b></td></tr>`

			// Abrir el archivo para leer los EBRs
			file, err := os.Open(diskPath)
			if err != nil {
				return fmt.Errorf("error abriendo el archivo del disco: %v", err)
			}
			defer file.Close()

			var ebr structures.EBR
			offset := part.Part_start
			for {
				// Moverse al inicio del EBR
				file.Seek(int64(offset), os.SEEK_SET)
				err := binary.Read(file, binary.LittleEndian, &ebr)
				if err != nil || ebr.Part_size <= 0 {
					break
				}

				// Convertir atributos a string sin caracteres nulos
				logicalName := strings.TrimRight(string(ebr.Part_name[:]), "\x00")
				logicalFit := rune(ebr.Part_fit[0])

				// Agregar la partición lógica a la tabla
				dotContent += fmt.Sprintf(`
					<tr><td bgcolor="lightgreen"><b>EBR</b></td><td bgcolor="lightgreen"><b>Partición Lógica</b></td></tr>
                <tr><td bgcolor="lightgray"><b>part_start</b></td><td>%d</td></tr>
                <tr><td bgcolor="lightgray"><b>part_size</b></td><td>%d</td></tr>
                <tr><td bgcolor="lightgray"><b>part_fit</b></td><td>%c</td></tr>
                <tr><td bgcolor="lightgray"><b>part_name</b></td><td>%s</td></tr>
				`, ebr.Part_start, ebr.Part_size, logicalFit, logicalName)

				// Pasar al siguiente EBR
				if ebr.Part_next <= 0 || ebr.Part_next >= mbr.Mbr_size {
					break
				}
				
				offset = ebr.Part_next
			}
		}
	}

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

	fmt.Println("Reporte MBR generado:", outputImage)
	return nil
}
