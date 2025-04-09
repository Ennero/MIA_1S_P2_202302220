package reports

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	structures "backend/structures"
	utils "backend/utils"
)

func ReportTree(sb *structures.SuperBlock, diskPath string, outputPath string) error {
	fmt.Printf("Generando reporte TREE en: %s\n", outputPath)

	err := utils.CreateParentDirs(outputPath)
	if err != nil {
		return fmt.Errorf("error creando directorios padre para reporte TREE: %v", err)
	}
	dotFileName, outputSVG := utils.GetFileNames(outputPath)

	// Maps para evitar duplicados
	generatedNodes := make(map[string]bool)
	generatedEdges := make(map[string]bool)

	var dotContent strings.Builder
	dotContent.WriteString("digraph FileSystemTree {\n")
	dotContent.WriteString("\trankdir=LR;\n")                 
	dotContent.WriteString("\tnode [shape=none, margin=0];\n") // Usaré labels HTML

	// Recorrer el árbol de inodos
	err = generateTreeRecursive(0, sb, diskPath, &dotContent, generatedNodes, generatedEdges)
	if err != nil {
		fmt.Printf("Advertencia durante la generación del árbol: %v\n", err)
		return fmt.Errorf("error generando el árbol de inodos: %v", err)
	}
	dotContent.WriteString("}\n")

	// Guardar y Ejecutar Graphviz 
	dotFile, err := os.Create(dotFileName)
	if err != nil {
		return fmt.Errorf("error al crear el archivo DOT para reporte TREE: %v", err)
	}
	defer dotFile.Close()

	_, err = dotFile.WriteString(dotContent.String())
	if err != nil {
		return fmt.Errorf("error al escribir en el archivo DOT para reporte TREE: %v", err)
	}
	fmt.Println("Archivo DOT generado:", dotFileName)

	cmd := exec.Command("dot", "-Tsvg", dotFileName, "-o", outputSVG)
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Salida de Graphviz:\n%s\n", string(cmdOutput))
		return fmt.Errorf("error al ejecutar Graphviz para reporte TREE: %v", err)
	}

	fmt.Println("Reporte TREE generado exitosamente en formato SVG:", outputSVG)
	return nil
}

// Llamado recursivo para generar el árbol de inodos
func generateTreeRecursive(
	inodeIndex int32,
	sb *structures.SuperBlock,
	diskPath string,
	dotContent *strings.Builder,
	generatedNodes map[string]bool,
	generatedEdges map[string]bool,
) error {
	inodeNodeID := fmt.Sprintf("inode_%d", inodeIndex)

	// Revisar si el inodo ya fue procesado
	if inodeIndex < 0 || inodeIndex >= sb.S_inodes_count {
		return fmt.Errorf("índice de inodo inválido %d encontrado", inodeIndex)
	}
	if generatedNodes[inodeNodeID] {
		return nil // Already processed this inode
	}

	// Marcar el inodo como generado
	generatedNodes[inodeNodeID] = true
	inode := &structures.Inode{}
	inodeOffset := int64(sb.S_inode_start) + int64(inodeIndex)*int64(sb.S_inode_size)
	if err := inode.Deserialize(diskPath, inodeOffset); err != nil {
		fmt.Printf("Error deserializando inodo %d: %v. Saltando.\n", inodeIndex, err)

		// Solo coloco un mensaje de error y un nodo de error en el DOT y sigo
		dotContent.WriteString(fmt.Sprintf("\t%s [label=\"Error Inodo %d\", shape=box, style=filled, fillcolor=red];\n", inodeNodeID, inodeIndex))
		return nil
	}

	// 3. Generate DOT node for the Inode
	inodeLabel := createInodeLabel(inodeIndex, inode)
	dotContent.WriteString(fmt.Sprintf("\t%s [label=<\n%s\n>];\n", inodeNodeID, inodeLabel))

	// 4. Process Inode pointers (I_block)
	for k := 0; k < 15; k++ {
		blockPtr := inode.I_block[k]
		if blockPtr == -1 {
			continue // Pointer not used
		}
		if blockPtr < 0 || blockPtr >= sb.S_blocks_count {
			fmt.Printf("Puntero de bloque inválido (%d) encontrado en inodo %d, i_block[%d]. Saltando.\n", blockPtr, inodeIndex, k)
			continue
		}
		blockNodeID := fmt.Sprintf("block_%d", blockPtr)
		inodePort := fmt.Sprintf("p%d", k)                                        // Port del inodo
		edgeID := fmt.Sprintf("%s:%s -> %s", inodeNodeID, inodePort, blockNodeID) // ID de la arista
		// Genera la arista entre el inodo y el bloque
		if !generatedEdges[edgeID] {
			dotContent.WriteString(fmt.Sprintf("\t%s:%s -> %s [label=\"i_block[%d]\"];\n", inodeNodeID, inodePort, blockNodeID, k))
			generatedEdges[edgeID] = true
		}
		// Genera el bloque del Inodo
		if !generatedNodes[blockNodeID] {
			generatedNodes[blockNodeID] = true // Se marca como ya generado
			blockOffset := int64(sb.S_block_start) + int64(blockPtr)*int64(sb.S_block_size)
			// Determina el tipo de bloque
			switch {
			case k < 12: // Bloques directos porque no tengo indirectos
				if inode.I_type[0] == '0' { // Folder Block
					folderBlock := &structures.FolderBlock{}
					if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
						fmt.Printf("Error deserializando FolderBlock %d: %v\n", blockPtr, err)
						dotContent.WriteString(fmt.Sprintf("\t%s [label=\"Error FolderBlock %d\", shape=box, style=filled, fillcolor=red];\n", blockNodeID, blockPtr))
					} else {
						label := createFolderBlockLabel(blockPtr, folderBlock)
						dotContent.WriteString(fmt.Sprintf("\t%s [label=<\n%s\n>];\n", blockNodeID, label))
						//Recursividad para procesar los hijos del bloque de carpeta
						for entryIdx, content := range folderBlock.B_content {
							name := strings.TrimRight(string(content.B_name[:]), "\x00")
							if content.B_inodo != -1 && name != "." && name != ".." {
								childInodeIndex := content.B_inodo
								childInodeNodeID := fmt.Sprintf("inode_%d", childInodeIndex)
								folderPort := fmt.Sprintf("i%d", entryIdx)
								entryEdgeID := fmt.Sprintf("%s:%s -> %s", blockNodeID, folderPort, childInodeNodeID)
								if !generatedEdges[entryEdgeID] {
									dotContent.WriteString(fmt.Sprintf("\t%s:%s -> %s [label=\"%s\"];\n", blockNodeID, folderPort, childInodeNodeID, name))
									generatedEdges[entryEdgeID] = true
								}
								// Recursividad para procesar el inodo hijo
								err := generateTreeRecursive(childInodeIndex, sb, diskPath, dotContent, generatedNodes, generatedEdges)
								if err != nil {
									fmt.Printf("Error en subárbol de inodo %d (desde bloque %d): %v\n", childInodeIndex, blockPtr, err)
								}
							}
						}

					}
				} else { // File Block
					fileBlock := &structures.FileBlock{}
					if err := fileBlock.Deserialize(diskPath, blockOffset); err != nil {
						fmt.Printf("Error deserializando FileBlock %d: %v\n", blockPtr, err)
						dotContent.WriteString(fmt.Sprintf("\t%s [label=\"Error FileBlock %d\", shape=box, style=filled, fillcolor=red];\n", blockNodeID, blockPtr))
					} else {
						label := createFileBlockLabel(blockPtr, fileBlock)
						dotContent.WriteString(fmt.Sprintf("\t%s [label=<\n%s\n>];\n", blockNodeID, label))
					}
				}
				// Bloques directos terminan aquí------------------------------------------------------------------------------------------------------------
			case k == 12:
				pointerBlock := &structures.PointerBlock{}
				if err := pointerBlock.Deserialize(diskPath, blockOffset); err != nil {
					fmt.Printf("Error deserializando PointerBlock %d (indirecto simple): %v\n", blockPtr, err)
					dotContent.WriteString(fmt.Sprintf("\t%s [label=\"Error PointerBlock %d\", shape=box, style=filled, fillcolor=red];\n", blockNodeID, blockPtr))
				} else {
					label := createPointerBlockLabel(blockPtr, pointerBlock)
					dotContent.WriteString(fmt.Sprintf("\t%s [label=<\n%s\n>];\n", blockNodeID, label))
					// Apuntador a bloques de datos
					for ptrIdx, dataBlockPtr := range pointerBlock.P_pointers {
						if dataBlockPtr == -1 {
							continue
						}
						if dataBlockPtr < 0 || dataBlockPtr >= sb.S_blocks_count {
							fmt.Printf("Puntero de bloque inválido (%d) encontrado en PointerBlock %d, P_pointers[%d]. Saltando.\n", dataBlockPtr, blockPtr, ptrIdx)
							continue
						}
						dataBlockNodeID := fmt.Sprintf("block_%d", dataBlockPtr)
						pointerPort := fmt.Sprintf("ptr%d", ptrIdx) // Puerto del bloque puntero
						ptrEdgeID := fmt.Sprintf("%s:%s -> %s", blockNodeID, pointerPort, dataBlockNodeID)
						// Añade la arista entre el bloque puntero y el bloque de datos
						if !generatedEdges[ptrEdgeID] {
							dotContent.WriteString(fmt.Sprintf("\t%s:%s -> %s [label=\"ptr[%d]\"];\n", blockNodeID, pointerPort, dataBlockNodeID, ptrIdx))
							generatedEdges[ptrEdgeID] = true
						}
						err := ensureBlockNodeExists(dataBlockPtr, inode.I_type[0], sb, diskPath, dotContent, generatedNodes, generatedEdges)
						if err != nil {
							fmt.Printf("Error asegurando nodo para bloque de datos %d (desde bloque puntero %d): %v\n", dataBlockPtr, blockPtr, err)
						}
					}
				}
				// Aquí sería lo de los bloques indirectos dobles y triples pero no lo tengo
			default:
				// Handle double/triple indirect if necessary
				dotContent.WriteString(fmt.Sprintf("\t%s [label=\"PointerBlock %d (Indirecto >1)\", shape=box, style=filled, fillcolor=lightgrey];\n", blockNodeID, blockPtr))
			}
		}
	}
	return nil
}

// Función para asegurarse que le bloque existe y generar su nodo que se usa para bloques indirectos
func ensureBlockNodeExists(
	blockIndex int32,
	originalInodeType byte, // Tipo original del inodo (0=folder, 1=file)
	sb *structures.SuperBlock,
	diskPath string,
	dotContent *strings.Builder,
	generatedNodes map[string]bool,
	generatedEdges map[string]bool, // Pasa también los edges generados
) error {

	//No existe
	blockNodeID := fmt.Sprintf("block_%d", blockIndex)
	if generatedNodes[blockNodeID] {
		return nil
	}
	generatedNodes[blockNodeID] = true // Marca que ya

	blockOffset := int64(sb.S_block_start) + int64(blockIndex)*int64(sb.S_block_size)

	// Genera el bloque del Inodo
	if originalInodeType == '0' { // Folder Block
		folderBlock := &structures.FolderBlock{}
		if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
			fmt.Printf("Error deserializando FolderBlock %d (indirecto): %v\n", blockIndex, err)
			dotContent.WriteString(fmt.Sprintf("\t%s [label=\"Error FolderBlock %d\", shape=box, style=filled, fillcolor=red];\n", blockNodeID, blockIndex))
			return err
		}
		label := createFolderBlockLabel(blockIndex, folderBlock)
		dotContent.WriteString(fmt.Sprintf("\t%s [label=<\n%s\n>];\n", blockNodeID, label))

		// Si es un bloque de carpeta, procesar sus entradas
		for entryIdx, content := range folderBlock.B_content {
			if content.B_inodo != -1 && string(content.B_name[:]) != "." && string(content.B_name[:]) != ".." {
				childInodeIndex := content.B_inodo
				childInodeNodeID := fmt.Sprintf("inode_%d", childInodeIndex)
				folderPort := fmt.Sprintf("i%d", entryIdx)
				entryEdgeID := fmt.Sprintf("%s:%s -> %s", blockNodeID, folderPort, childInodeNodeID)

				if !generatedEdges[entryEdgeID] {
					entryName := strings.TrimRight(string(content.B_name[:]), "\x00")
					dotContent.WriteString(fmt.Sprintf("\t%s:%s -> %s [label=\"%s\"];\n", blockNodeID, folderPort, childInodeNodeID, entryName))
					generatedEdges[entryEdgeID] = true
				}
				// Verifica si el inodo hijo ya fue generado
				err := generateTreeRecursive(childInodeIndex, sb, diskPath, dotContent, generatedNodes, generatedEdges)
				if err != nil {
					fmt.Printf("Error en subárbol de inodo %d (desde bloque %d indirecto): %v\n", childInodeIndex, blockIndex, err)
				}
			}
		}

	} else { // File Block
		fileBlock := &structures.FileBlock{}
		if err := fileBlock.Deserialize(diskPath, blockOffset); err != nil {
			fmt.Printf("Error deserializando FileBlock %d (indirecto): %v\n", blockIndex, err)
			dotContent.WriteString(fmt.Sprintf("\t%s [label=\"Error FileBlock %d\", shape=box, style=filled, fillcolor=red];\n", blockNodeID, blockIndex))
			return err
		}
		label := createFileBlockLabel(blockIndex, fileBlock)
		dotContent.WriteString(fmt.Sprintf("\t%s [label=<\n%s\n>];\n", blockNodeID, label))
	}
	return nil
}

// Genera la etiqueta HTML para el inodo
func createInodeLabel(index int32, inode *structures.Inode) string {
	var label strings.Builder
	label.WriteString("<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"4\">\n")
	label.WriteString(fmt.Sprintf("<TR><TD COLSPAN=\"2\" BGCOLOR=\"lightblue\"><B>Inodo %d</B></TD></TR>\n", index))
	label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\">I_UID</TD><TD ALIGN=\"LEFT\">%d</TD></TR>\n", inode.I_uid))
	label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\">I_GID</TD><TD ALIGN=\"LEFT\">%d</TD></TR>\n", inode.I_gid))
	label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\">I_SIZE</TD><TD ALIGN=\"LEFT\">%d</TD></TR>\n", inode.I_size))
	atime := time.Unix(int64(inode.I_atime), 0).Format("2006-01-02 15:04:05")
	ctime := time.Unix(int64(inode.I_ctime), 0).Format("2006-01-02 15:04:05")
	mtime := time.Unix(int64(inode.I_mtime), 0).Format("2006-01-02 15:04:05")
	label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\">I_ATIME</TD><TD ALIGN=\"LEFT\">%s</TD></TR>\n", atime))
	label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\">I_CTIME</TD><TD ALIGN=\"LEFT\">%s</TD></TR>\n", ctime))
	label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\">I_MTIME</TD><TD ALIGN=\"LEFT\">%s</TD></TR>\n", mtime))
	typeStr := "Archivo ('1')"
	if inode.I_type[0] == '0' {
		typeStr = "Directorio ('0')"
	}
	label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\">I_TYPE</TD><TD ALIGN=\"LEFT\">%s</TD></TR>\n", typeStr))
	label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\">I_PERM</TD><TD ALIGN=\"LEFT\">%s</TD></TR>\n", string(inode.I_perm[:])))
	for i := 0; i < 15; i++ {
		label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\" PORT=\"p%d\">I_BLOCK[%d]</TD><TD ALIGN=\"LEFT\">%d</TD></TR>\n", i, i, inode.I_block[i]))
	}
	label.WriteString("</TABLE>")
	return label.String()
}

// Genera la etiqueta HTML para el bloque de carpeta
func createFolderBlockLabel(index int32, block *structures.FolderBlock) string {
	var label strings.Builder
	label.WriteString("<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"4\">\n")
	label.WriteString(fmt.Sprintf("<TR><TD COLSPAN=\"3\" BGCOLOR=\"lightcoral\"><B>FolderBlock %d</B></TD></TR>\n", index))
	label.WriteString("<TR><TD><B>Index</B></TD><TD><B>Name</B></TD><TD><B>Inode Ptr</B></TD></TR>\n")
	for i, content := range block.B_content {
		name := strings.TrimRight(string(content.B_name[:]), "\x00")
		label.WriteString(fmt.Sprintf("<TR><TD>%d</TD><TD PORT=\"i%d\">%s</TD><TD>%d</TD></TR>\n", i, i, name, content.B_inodo))
	}
	label.WriteString("</TABLE>")
	return label.String()
}

// Genera la etiqueta HTML para el bloque de archivo
func createFileBlockLabel(index int32, block *structures.FileBlock) string {
	var label strings.Builder
	label.WriteString("<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"4\">\n")
	label.WriteString(fmt.Sprintf("<TR><TD BGCOLOR=\"lightgoldenrodyellow\"><B>FileBlock %d</B></TD></TR>\n", index))

	// Obtener contenido y limpiar nulls explícitamente por seguridad
	contentPreview := string(block.B_content[:])
	contentPreview = strings.TrimRight(contentPreview, "\x00") // Limpiar nulls al final

	maxLen := 40 // Aumentado ligeramente el límite
	if len(contentPreview) > maxLen {
		contentPreview = contentPreview[:maxLen] + "..."
	}

	// Capeando un par de caracteres especiales
	contentPreview = strings.ReplaceAll(contentPreview, "&", "&amp;")
	contentPreview = strings.ReplaceAll(contentPreview, "<", "&lt;")
	contentPreview = strings.ReplaceAll(contentPreview, ">", "&gt;")
	contentPreview = strings.ReplaceAll(contentPreview, "\n", " ") // Reemplazar saltos de línea por espacios
	contentPreview = strings.TrimSuffix(contentPreview, " ")
	// Añadir el contenido procesado a la etiqueta, alineado a la izquierda
	label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\">%s</TD></TR>\n", contentPreview))
	label.WriteString("</TABLE>")
	return label.String()
}

// Genera la etiqueta HTML para el bloque puntero
func createPointerBlockLabel(index int32, block *structures.PointerBlock) string {
	var label strings.Builder
	label.WriteString("<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"4\">\n")
	label.WriteString(fmt.Sprintf("<TR><TD COLSPAN=\"2\" BGCOLOR=\"lightgrey\"><B>PointerBlock %d</B></TD></TR>\n", index))
	for i, ptr := range block.P_pointers {
		label.WriteString(fmt.Sprintf("<TR><TD ALIGN=\"LEFT\" PORT=\"ptr%d\">P_POINTER[%d]</TD><TD ALIGN=\"LEFT\">%d</TD></TR>\n", i, i, ptr))
	}
	label.WriteString("</TABLE>")
	return label.String()
}
