package reports

import (
	structures "backend/structures"
	utils "backend/utils"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ReporteBloque genera un reporte detallado de los bloques usados,
// evitando duplicados y conectándolos secuencialmente según se descubren.
func ReportBlock(superblock *structures.SuperBlock, diskPath string, path string) error {
	err := utils.CreateParentDirs(path)
	if err != nil {
		return err
	}
	dotFileName, outputImage := utils.GetFileNames(path)

	// --- Leer Bitmap de Inodos (Necesario si S_inodes_count es total) ---
	inodeBitmapSize := superblock.S_inodes_count
	if inodeBitmapSize <= 0 {
		return fmt.Errorf("s_inodes_count inválido: %d", inodeBitmapSize)
	}
	inodeBitmap := make([]byte, inodeBitmapSize)
	file, err := os.Open(diskPath)
	if err != nil {
		return fmt.Errorf("error al abrir disco para leer bitmap de inodos: %w", err)
	}
	_, err = file.Seek(int64(superblock.S_bm_inode_start), 0)
	if err != nil {
		file.Close()
		return fmt.Errorf("error al buscar inicio de bitmap de inodos: %w", err)
	}
	bytesRead, err := file.Read(inodeBitmap)
	file.Close()
	if err != nil || int32(bytesRead) != inodeBitmapSize {
		return fmt.Errorf("error al leer bitmap de inodos completo: %w", err)
	}
	// --- Fin Lectura Bitmap ---

	generatedBlockNodes := make(map[int32]bool) // Evitar nodos duplicados
	var lastValidBlockIndex int32 = -1          // Para enlace secuencial de descubrimiento

	dotContent := `digraph G {
        rankdir=LR;
        node [shape=plaintext];
    `

	// Iterar sobre los posibles slots de inodo
	for i := int32(0); i < superblock.S_inodes_count; i++ {

		// Filtrar: Solo procesar inodos usados
		if inodeBitmap[i] != '1' {
			continue
		}

		// Inodo 'i' está usado
		inode := &structures.Inode{}
		inodeOffset := int64(superblock.S_inode_start + (i * superblock.S_inode_size))
		err := inode.Deserialize(diskPath, inodeOffset)
		if err != nil {
			fmt.Printf("Error deserializando inodo %d para reporte de bloques: %v. Saltando inodo.\n", i, err)
			// Podríamos generar un nodo inodo de error si quisiéramos verlo
			// dotContent += fmt.Sprintf("\tinode_err%d [label=\"Error Inodo %d\"];\n", i, i)
			continue // Saltar al siguiente inodo
		}

		// Iterar sobre los punteros de bloque del inodo actual 'i'
		// Usamos 'k' para saber si es directo, indirecto, etc.
		for k, blockPtr := range inode.I_block {
			if blockPtr == -1 {
				continue
			} // Puntero no usado
			if blockPtr < 0 || blockPtr >= superblock.S_blocks_count {
				fmt.Printf("Puntero de bloque inválido (%d) encontrado en inodo %d, i_block[%d]. Saltando.\n", blockPtr, i, k)
				continue // Saltar puntero inválido
			}

			// --- Lógica de Secuencia y Duplicados ---
			isNewNode := !generatedBlockNodes[blockPtr]

			if isNewNode {
				// Conectar al bloque VÁLIDO anterior ANTES de generar el nuevo nodo
				if lastValidBlockIndex != -1 {
					dotContent += fmt.Sprintf("\n\tblock%d -> block%d;", lastValidBlockIndex, blockPtr)
				}
				// Actualizar cuál fue el último bloque único procesado
				lastValidBlockIndex = blockPtr
				// Marcar este bloque como ya generado para no repetirlo
				generatedBlockNodes[blockPtr] = true
			} else {
				// Si el nodo ya existe, no hacemos nada más para este puntero
				continue
			}
			// --- Fin Lógica Secuencia/Duplicados ---

			// --- Generación del Nodo Bloque Detallado (Solo si isNewNode es true) ---
			blockNodeID := fmt.Sprintf("block%d", blockPtr)
			blockOffset := int64(superblock.S_block_start + (blockPtr * superblock.S_block_size))
			var blockLabel string = fmt.Sprintf("Error Block %d", blockPtr) // Default
			var blockGenerated bool = false                                 // Flag para saber si se generó una definición válida

			// Determinar tipo y generar label
			// Necesitamos distinguir bloques de datos de bloques de apuntadores
			blockIsPointer := k >= 12 // Índices 12, 13, 14 son para apuntadores

			if blockIsPointer {
				// Bloque de Apuntadores (Simple, Doble, Triple)
				block := &structures.PointerBlock{}
				err := block.Deserialize(diskPath, blockOffset)
				if err == nil {
					var label strings.Builder
					label.WriteString(`<table border="0" cellborder="1" cellspacing="0" cellpadding="4">`)
					bgColor := "lightgrey"
					title := "Bloque Apuntadores"
					if k == 13 {
						bgColor = "grey"
						title = "Bloque Ap. Doble"
					}
					if k == 14 {
						bgColor = "darkgrey"
						title = "Bloque Ap. Triple"
					}
					label.WriteString(fmt.Sprintf(`<tr><td colspan="2" bgcolor="%s"><b>%s %d</b></td></tr>`, bgColor, title, blockPtr))
					label.WriteString(`<tr><td bgcolor="lightyellow"><b>Índice</b></td><td bgcolor="lightyellow"><b>Bloque Apuntado</b></td></tr>`)
					for ptrIdx, ptrVal := range block.P_pointers {
						label.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%d</td></tr>`, ptrIdx, ptrVal))
					}
					label.WriteString(`</table>`)
					blockLabel = label.String()
					blockGenerated = true
				} else {
					fmt.Printf("Error deserializando Bloque Apuntadores %d: %v\n", blockPtr, err)
				}

			} else {
				// Bloque de Datos (Carpeta o Archivo) - según inode.I_type
				switch inode.I_type[0] {
				case '0': // Carpeta
					block := &structures.FolderBlock{}
					err := block.Deserialize(diskPath, blockOffset)
					if err == nil {
						var label strings.Builder
						label.WriteString(`<table border="0" cellborder="1" cellspacing="0" cellpadding="4">`)
						label.WriteString(fmt.Sprintf(`<tr><td colspan="2" bgcolor="lightcoral"><b>Bloque Carpeta %d</b></td></tr>`, blockPtr))
						label.WriteString(`<tr><td bgcolor="lightgreen"><b>Nombre</b></td><td bgcolor="lightgreen"><b>Inodo Ptr</b></td></tr>`)
						for _, content := range block.B_content {
							name := string(bytes.TrimRight(content.B_name[:], "\x00"))
							name = strings.ReplaceAll(name, "&", "&amp;")
							name = strings.ReplaceAll(name, "<", "&lt;")
							name = strings.ReplaceAll(name, ">", "&gt;")
							label.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`, name, content.B_inodo))
						}
						label.WriteString(`</table>`)
						blockLabel = label.String()
						blockGenerated = true
					} else {
						fmt.Printf("Error deserializando Bloque Carpeta %d: %v\n", blockPtr, err)
					}

				case '1': // Archivo
					block := &structures.FileBlock{}
					err := block.Deserialize(diskPath, blockOffset)
					if err == nil {
						content := string(bytes.TrimRight(block.B_content[:], "\x00"))
						content = strings.ReplaceAll(content, "&", "&amp;")
						content = strings.ReplaceAll(content, "<", "&lt;")
						content = strings.ReplaceAll(content, ">", "&gt;")
						content = strings.ReplaceAll(content, "\n", "<BR/>")
						blockLabel = fmt.Sprintf(`<table border="0" cellborder="1" cellspacing="0" cellpadding="4">
                            <tr><td bgcolor="lightgoldenrodyellow"><b>Bloque Archivo %d</b></td></tr>
                            <tr><td align="left">%s</td></tr>
                        </table>`, blockPtr, content)
						blockGenerated = true
					} else {
						fmt.Printf("Error deserializando Bloque Archivo %d: %v\n", blockPtr, err)
					}
				default:
					fmt.Printf("Tipo de inodo desconocido '%c' para bloque de datos %d\n", inode.I_type[0], blockPtr)
					blockLabel = fmt.Sprintf("Bloque %d (Tipo Inodo Desconocido)", blockPtr)
				} // End switch inode.I_type
			} // End else (Bloque de Datos)

			// Añadir la definición del nodo bloque solo si se generó correctamente
			if blockGenerated {
				dotContent += fmt.Sprintf("\n\t%s [label=<%s>];", blockNodeID, blockLabel)
			} else {
				// Si hubo error de deserialización, usar el label de error
				dotContent += fmt.Sprintf("\n\t%s [label=\"%s\", shape=box, style=filled, fillcolor=red];", blockNodeID, blockLabel)
				// No conectar desde/hacia nodos de error, resetear lastValidBlockIndex
				lastValidBlockIndex = -1
			}
			// --- Fin Generación Nodo Detallado ---

		} // Fin bucle interior blockPtr
	} // Fin bucle exterior i

	dotContent += "\n}" // Cerrar grafo

	// --- El resto (escribir DOT, ejecutar dot) sin cambios ---
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
		fmt.Printf("Error ejecutando Graphviz para ReportBlock:\n%s\n", string(cmdOutput))
		return fmt.Errorf("error al ejecutar Graphviz: %w", err)
	}

	fmt.Println("Imagen de los bloques generada:", outputImage)
	return nil
}
