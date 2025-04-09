package reports

import (
	structures "backend/structures"
	utils "backend/utils"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"sort" // Necesario para ordenar las particiones
	"strings"
)

type PartitionInfo struct {
	Partition structures.Partition
	Index     int
}

func ReportDisk(mbr *structures.MBR, diskPath string, outputPath string) error {
	err := utils.CreateParentDirs(outputPath)
	if err != nil {
		return err
	}
	totalSize := mbr.Mbr_size
	name := utils.GetDiskName(diskPath)

	dotFileName, outputImage := utils.GetFileNames(outputPath)

	// Recolectar y ordenar particiones válidas (primarias y extendida) por offset
	validPartitions := []PartitionInfo{}
	for i := 0; i < 4; i++ {
		part := mbr.Mbr_partitions[i]
		// Considerar solo particiones con tamaño positivo
		if part.Part_size > 0 {
			validPartitions = append(validPartitions, PartitionInfo{Partition: part, Index: i})
		}
	}
	// Ordenar por Part_start
	sort.Slice(validPartitions, func(i, j int) bool {
		return validPartitions[i].Partition.Part_start < validPartitions[j].Partition.Part_start
	})

	// Construir el contenido DOT iterando por el espacio del disco
	dotContent := "digraph G {\n"
	dotContent += "\tnode [shape=none];\n"
	dotContent += "\tgraph [splines=false];\n"
	dotContent += "\tsubgraph cluster_disk {\n"
	dotContent += fmt.Sprintf("\t\tlabel=\"Disco: %s (Tamaño Total: %d bytes)\";\n", name, totalSize) 
	dotContent += "\t\tstyle=filled;\n"
	dotContent += "\t\tfillcolor=white;\n"
	dotContent += "\t\tcolor=black;\n"
	dotContent += "\t\tpenwidth=2;\n"

	dotContent += "\t\ttable [label=<\n\t\t\t<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"10\" WIDTH=\"800\">\n" 
	dotContent += "\t\t\t<TR>\n"

	// Calcular el tamaño del MBR para el offset inicial.
	mbrStructSize := int32(binary.Size(*mbr)) // Tamaño real de la estructura MBR
	dotContent += fmt.Sprintf("\t\t\t<TD BGCOLOR=\"gray\" ALIGN=\"CENTER\"><B>MBR</B><BR/>%d bytes</TD>\n", mbrStructSize)

	lastOffset := int64(mbrStructSize) 

	ebrStructSize := int32(binary.Size(structures.EBR{})) // Tamaño de la estructura EBR

	// Iterar sobre las particiones ordenadas
	for _, pInfo := range validPartitions {
		part := pInfo.Partition

		// Calcular espacio libre ANTES de esta partición
		freeSpaceBefore := int64(part.Part_start) - lastOffset
		if freeSpaceBefore < 0 {
			// Esto indica un solapamiento o un MBR más grande de lo esperado, ajustar
			fmt.Printf("Advertencia: Posible solapamiento detectado antes de partición '%s'. Espacio libre antes = %d. Ajustando lastOffset.\n", strings.TrimRight(string(part.Part_name[:]), "\x00"), freeSpaceBefore)
			freeSpaceBefore = 0                 
		}

		if freeSpaceBefore > 0 {
			freePercentage := float64(freeSpaceBefore) / float64(totalSize) * 100
			cellWidth := int(float64(freeSpaceBefore) / float64(totalSize) * 800) 
			if cellWidth < 30 {
				cellWidth = 30
			} // Ancho mínimo para legibilidad
			dotContent += fmt.Sprintf("\t\t\t<TD BGCOLOR=\"#F5F5F5\" WIDTH=\"%d\" ALIGN=\"CENTER\"><B>Libre</B><BR/>%d bytes<BR/>(%.2f%%)</TD>\n",
				cellWidth, freeSpaceBefore, freePercentage)
		}

		// Procesar la partición actual
		percentage := float64(part.Part_size) / float64(totalSize) * 100
		cellWidth := int(float64(part.Part_size) / float64(totalSize) * 800) // Ancho proporcional
		if cellWidth < 50 {
			cellWidth = 50
		}
		partName := strings.TrimRight(string(part.Part_name[:]), "\x00")

		switch part.Part_type[0] {
		case 'P': // Partición Primaria
			dotContent += fmt.Sprintf("\t\t\t<TD BGCOLOR=\"lightblue\" WIDTH=\"%d\" ALIGN=\"CENTER\"><B>Primaria</B><BR/>%s<BR/>%d bytes<BR/>(%.2f%%)</TD>\n",
				cellWidth, partName, part.Part_size, percentage)

		case 'E': // Partición Extendida
			dotContent += fmt.Sprintf("\t\t\t<TD BGCOLOR=\"lightcoral\" WIDTH=\"%d\" ALIGN=\"CENTER\" CELLPADDING=\"0\">\n", cellWidth)
			dotContent += "\t\t\t\t<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"5\" WIDTH=\"100%\" HEIGHT=\"100%\">\n"
			dotContent += fmt.Sprintf("\t\t\t\t<TR><TD COLSPAN=\"100\" ALIGN=\"CENTER\" BGCOLOR=\"orange\"><B>Extendida: %s (%d bytes)</B></TD></TR>\n", partName, part.Part_size) 
			dotContent += "\t\t\t\t<TR>\n"

			file, err := os.Open(diskPath)
			if err != nil {
				dotContent += fmt.Sprintf("\t\t\t\t<TD BGCOLOR=\"red\" ALIGN=\"CENTER\">Error abriendo disco: %v</TD>\n", err)
			} else {
				defer file.Close() 

				var ebr structures.EBR
				currentEbrOffset := int64(part.Part_start) 
				lastElementEndInE := currentEbrOffset    

				for {
					file.Seek(currentEbrOffset, os.SEEK_SET)
					err := binary.Read(file, binary.LittleEndian, &ebr)
					if err != nil {
						fmt.Printf("Fin de cadena EBR o error de lectura en offset %d: %v\n", currentEbrOffset, err)
						break 
					}

					// Calcular espacio libre ANTES de este EBR
					freeSpaceBeforeEbr := currentEbrOffset - lastElementEndInE
					if freeSpaceBeforeEbr > 0 {
						freeExtPercentage := float64(freeSpaceBeforeEbr) / float64(totalSize) * 100
						cellFreeExtWidth := int(float64(freeSpaceBeforeEbr) / float64(totalSize) * 800)
						if cellFreeExtWidth < 30 {
							cellFreeExtWidth = 30
						}
						dotContent += fmt.Sprintf("\t\t\t\t<TD BGCOLOR=\"#D3D3D3\" WIDTH=\"%d\" ALIGN=\"CENTER\"><B>Libre Ext.</B><BR/>%d bytes<BR/>(%.2f%%)</TD>\n",
							cellFreeExtWidth, freeSpaceBeforeEbr, freeExtPercentage)
					} else if freeSpaceBeforeEbr < 0 {
						fmt.Printf("Advertencia: Solapamiento detectado dentro de extendida antes de EBR en %d (last_end=%d)\n", currentEbrOffset, lastElementEndInE)
					}

					cellEbrWidth := int(float64(ebrStructSize) / float64(totalSize) * 800)
					if cellEbrWidth < 20 {
						cellEbrWidth = 20
					} 
					dotContent += fmt.Sprintf("\t\t\t\t<TD BGCOLOR=\"gray\" WIDTH=\"%d\" ALIGN=\"CENTER\"><B>EBR</B></TD>\n", cellEbrWidth)
					lastElementEndInE = currentEbrOffset + int64(ebrStructSize) // Actualizar offset

					// Procesar la partición lógica asociada a este EBR (si existe y tiene tamaño)
					if ebr.Part_size > 0 {
						freeSpaceBeforeLogical := int64(ebr.Part_start) - lastElementEndInE
						if freeSpaceBeforeLogical > 0 {
							freeExtPercentage := float64(freeSpaceBeforeLogical) / float64(totalSize) * 100
							cellFreeExtWidth := int(float64(freeSpaceBeforeLogical) / float64(totalSize) * 800)
							if cellFreeExtWidth < 30 {
								cellFreeExtWidth = 30
							}
							dotContent += fmt.Sprintf("\t\t\t\t<TD BGCOLOR=\"#D3D3D3\" WIDTH=\"%d\" ALIGN=\"CENTER\"><B>Libre Ext.</B><BR/>%d bytes<BR/>(%.2f%%)</TD>\n",
								cellFreeExtWidth, freeSpaceBeforeLogical, freeExtPercentage)
						} else if freeSpaceBeforeLogical < 0 {
							fmt.Printf("Advertencia: Solapamiento detectado entre EBR en %d y Lógica en %d\n", currentEbrOffset, ebr.Part_start)
						}

						// Añadir celda para la Lógica
						logicalPercentage := float64(ebr.Part_size) / float64(totalSize) * 100
						logicalName := strings.TrimRight(string(ebr.Part_name[:]), "\x00")
						cellLogicalWidth := int(float64(ebr.Part_size) / float64(totalSize) * 800)
						if cellLogicalWidth < 50 {
							cellLogicalWidth = 50
						}
						dotContent += fmt.Sprintf("\t\t\t\t<TD BGCOLOR=\"lightgreen\" WIDTH=\"%d\" ALIGN=\"CENTER\"><B>Lógica</B><BR/>%s<BR/>%d bytes<BR/>(%.2f%%)</TD>\n",
							cellLogicalWidth, logicalName, ebr.Part_size, logicalPercentage)
						lastElementEndInE = int64(ebr.Part_start + ebr.Part_size) // Actualizar offset al final de la lógica
					} else {
						fmt.Printf("Info: EBR en offset %d tiene tamaño 0.\n", currentEbrOffset)
					}

					// Avanzar al siguiente EBR
					if ebr.Part_next <= 0 || int64(ebr.Part_next) <= currentEbrOffset { 
						fmt.Printf("Fin de cadena EBR (Part_next=%d).\n", ebr.Part_next)
						break
					}
					currentEbrOffset = int64(ebr.Part_next) 
				} 

				// Calcular espacio libre al FINAL de la partición extendida
				endOfExtended := int64(part.Part_start + part.Part_size)
				freeSpaceAtEnd := endOfExtended - lastElementEndInE
				if freeSpaceAtEnd > 0 {
					freeExtPercentage := float64(freeSpaceAtEnd) / float64(totalSize) * 100
					cellFreeExtWidth := int(float64(freeSpaceAtEnd) / float64(totalSize) * 800)
					if cellFreeExtWidth < 30 {
						cellFreeExtWidth = 30
					}
					dotContent += fmt.Sprintf("\t\t\t\t<TD BGCOLOR=\"#D3D3D3\" WIDTH=\"%d\" ALIGN=\"CENTER\"><B>Libre Ext.</B><BR/>%d bytes<BR/>(%.2f%%)</TD>\n",
						cellFreeExtWidth, freeSpaceAtEnd, freeExtPercentage)
				} else if freeSpaceAtEnd < 0 {
					fmt.Printf("Advertencia: El contenido de la extendida parece exceder su tamaño (end=%d, last_end=%d)\n", endOfExtended, lastElementEndInE)
				}

				dotContent += "\t\t\t\t</TR>\n"
				dotContent += "\t\t\t\t</TABLE>\n"
			} 
			dotContent += "\t\t\t</TD>\n" 

		default:
			dotContent += fmt.Sprintf("\t\t\t<TD BGCOLOR=\"pink\" WIDTH=\"%d\" ALIGN=\"CENTER\"><B>Tipo Desc: %c</B><BR/>%s<BR/>%d bytes<BR/>(%.2f%%)</TD>\n",
				cellWidth, part.Part_type[0], partName, part.Part_size, percentage)
		}

		// Actualizar el offset para el siguiente cálculo de espacio libre
		lastOffset = int64(part.Part_start + part.Part_size)

	}

	// Calcular espacio libre FINAL después de la última partición
	finalFreeSpace := int64(totalSize) - lastOffset
	if finalFreeSpace < 0 {
		fmt.Printf("Advertencia: El espacio ocupado calculado (%d) excede el tamaño total del disco (%d).\n", lastOffset, totalSize)
		finalFreeSpace = 0 
	}

	if finalFreeSpace > 0 {
		freePercentage := float64(finalFreeSpace) / float64(totalSize) * 100
		cellWidth := int(float64(finalFreeSpace) / float64(totalSize) * 800)
		if cellWidth < 30 {
			cellWidth = 30
		}
		dotContent += fmt.Sprintf("\t\t\t<TD BGCOLOR=\"#F5F5F5\" WIDTH=\"%d\" ALIGN=\"CENTER\"><B>Libre</B><BR/>%d bytes<BR/>(%.2f%%)</TD>\n",
			cellWidth, finalFreeSpace, freePercentage)
	}

	dotContent += "\t\t\t</TR>\n"
	dotContent += "\t\t\t</TABLE>\n>];\n" 
	dotContent += "\t}\n"      
	dotContent += "}\n"                   // Cierre 

	file, err := os.Create(dotFileName)
	if err != nil {
		return fmt.Errorf("error al crear el archivo DOT: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString(dotContent)
	if err != nil {
		return fmt.Errorf("error al escribir en el archivo DOT: %v", err)
	}

	cmd := exec.Command("dot", "-Tpng", dotFileName, "-o", outputImage)
	output, err := cmd.CombinedOutput() // Capturar salida estándar y de error
	if err != nil {
		fmt.Printf("Salida de Graphviz:\n%s\n", string(output))
		return fmt.Errorf("error al ejecutar Graphviz (dot): %v. Asegúrate que Graphviz esté instalado y en el PATH", err)
	}

	fmt.Println("Reporte DISK generado:", outputImage)
	return nil
}
