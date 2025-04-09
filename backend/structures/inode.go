package structures

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Inode struct {
	I_uid   int32
	I_gid   int32
	I_size  int32
	I_atime float32
	I_ctime float32
	I_mtime float32
	I_block [15]int32
	I_type  [1]byte
	I_perm  [3]byte
	// Total: 88 bytes
}

func (inode *Inode) Serialize(path string, offset int64) error {
	if offset < 0 {
		return fmt.Errorf("offset negativo inválido para serializar inodo: %d", offset)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("error abriendo archivo '%s' para escribir inodo en offset %d: %w", path, offset, err)
	}
	defer file.Close()

	_, err = file.Seek(offset, 0)
	if err != nil {
		return fmt.Errorf("error buscando offset %d para escribir inodo: %w", offset, err)
	}

	err = binary.Write(file, binary.LittleEndian, inode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo en offset %d con binary.Write: %w", offset, err)
	}

	return nil
}

// Deserialize lee la estructura Inode desde un archivo binario en la posición especificada
func (inode *Inode) Deserialize(path string, offset int64) error {
	if offset < 0 {
		return fmt.Errorf("offset negativo inválido para deserializar inodo: %d", offset)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("error abriendo archivo '%s' para leer inodo en offset %d: %w", path, offset, err)
	}
	defer file.Close()

	// Mover el puntero del archivo a la posición especificada
	_, err = file.Seek(offset, 0)
	if err != nil {
		// Podría ser un offset más allá del final del archivo si el índice es inválido
		return fmt.Errorf("error buscando offset %d para leer inodo: %w", offset, err)
	}

	// Usar un puntero a un Inode vacío para calcular el tamaño esperado
	expectedSize := binary.Size(&Inode{})
	if expectedSize <= 0 {
		return fmt.Errorf("tamaño calculado de estructura Inode inválido: %d", expectedSize)
	}

	// Leer solo la cantidad de bytes que corresponden al tamaño de la estructura Inode
	buffer := make([]byte, expectedSize)
	bytesRead, err := file.Read(buffer)

	// Manejar errores de lectura
	if err != nil {
		if err == io.EOF {
			if bytesRead == 0 {
				return fmt.Errorf("EOF alcanzado inmediatamente al intentar leer inodo en offset %d (posible offset inválido o archivo vacío?)", offset)
			}
			return fmt.Errorf("inodo truncado en offset %d: se leyeron %d bytes, se esperaban %d (%w)", offset, bytesRead, expectedSize, err)
		}
		return fmt.Errorf("error de I/O leyendo inodo en offset %d: %w", offset, err)
	}

	// Verificar si se leyó la cantidad exacta de bytes
	if bytesRead < expectedSize {
		return fmt.Errorf("lectura incompleta de inodo en offset %d: se leyeron %d bytes, se esperaban %d", offset, bytesRead, expectedSize)
	}

	// Deserializar los bytes leídos en la estructura Inode
	reader := bytes.NewReader(buffer)
	err = binary.Read(reader, binary.LittleEndian, inode)
	if err != nil {
		return fmt.Errorf("error deserializando bytes de inodo desde offset %d: %w", offset, err)
	}
	return nil
}

// Print imprime los atributos del inodo de forma legible
func (inode *Inode) Print() {

	atime := time.Unix(int64(inode.I_atime), 0)
	ctime := time.Unix(int64(inode.I_ctime), 0)
	mtime := time.Unix(int64(inode.I_mtime), 0)
	timeFormat := "2006-01-02 15:04:05"

	fmt.Printf("  I_uid: %d\n", inode.I_uid)
	fmt.Printf("  I_gid: %d\n", inode.I_gid)
	fmt.Printf("  I_size: %d bytes\n", inode.I_size)
	fmt.Printf("  I_atime: %s (%.0f)\n", atime.Format(timeFormat), inode.I_atime)
	fmt.Printf("  I_ctime: %s (%.0f)\n", ctime.Format(timeFormat), inode.I_ctime)
	fmt.Printf("  I_mtime: %s (%.0f)\n", mtime.Format(timeFormat), inode.I_mtime)
	fmt.Printf("  I_type: %c (%s)\n", inode.I_type[0], map[byte]string{'0': "Directorio", '1': "Archivo"}[inode.I_type[0]])
	fmt.Printf("  I_perm: %s\n", string(inode.I_perm[:]))
	fmt.Printf("  I_block Pointers:\n")
	fmt.Printf("    Directos [0-11] : %v\n", inode.I_block[0:12])
	fmt.Printf("    Indirecto L1 [12]: %d\n", inode.I_block[12])
	fmt.Printf("    Indirecto L2 [13]: %d\n", inode.I_block[13])
	fmt.Printf("    Indirecto L3 [14]: %d\n", inode.I_block[14])

}

// FUNCIÓN PARA BUSCAR UN ARCHIVO---------------------------------------------------------------------------------------
// FUNCIÓN PARA BUSCAR UN ARCHIVO---------------------------------------------------------------------------------------
func FindInodeByPath(sb *SuperBlock, diskPath string, path string) (int32, *Inode, error) {
	fmt.Printf("Buscando inodo para path: %s\n", path)

	components := strings.Split(path, "/")
	var cleanComponents []string
	for _, c := range components {
		if c != "" {
			cleanComponents = append(cleanComponents, c)
		}
	}

	fmt.Printf("Componentes del path: %v\n", cleanComponents)

	// Si es un path vacío o solo la raíz (/), devolver el inodo raíz
	if len(cleanComponents) == 0 {
		rootInode := &Inode{}
		if err := rootInode.Deserialize(diskPath, int64(sb.S_inode_start)); err != nil {
			return -1, nil, fmt.Errorf("error al leer inodo raíz: %v", err)
		}
		return 0, rootInode, nil
	}

	currentInodeNum := int32(0) // Inodo raíz es 0

	// Para cada componente del path, buscar en el directorio correspondiente
	for i, component := range cleanComponents {
		fmt.Printf("Buscando componente %d: %s (en inodo %d)\n", i, component, currentInodeNum)

		currentInode := &Inode{}
		offset := int64(sb.S_inode_start + currentInodeNum*sb.S_inode_size)
		if err := currentInode.Deserialize(diskPath, offset); err != nil {
			return -1, nil, err
		}

		fmt.Printf("Tipo de inodo actual: %s\n", string(currentInode.I_type[:]))

		// Verificar que el inodo actual es un directorio (excepto para el último componente)
		if i < len(cleanComponents)-1 && currentInode.I_type[0] != '0' {
			return -1, nil, fmt.Errorf("'%s' no es un directorio", component)
		}

		found := false
		// Iterar sobre los bloques de punteros del inodo actual
		for blockIndex, blockPtr := range currentInode.I_block {
			if blockPtr == -1 {
				continue
			}
			// Debugeando
			fmt.Printf("Examinando bloque %d (puntero %d) del inodo %d\n",
				blockIndex, blockPtr, currentInodeNum)

			// Leer el bloque de carpeta
			folderBlock := &FolderBlock{}
			blockOffset := int64(sb.S_block_start + blockPtr*sb.S_block_size)
			if err := folderBlock.Deserialize(diskPath, blockOffset); err != nil {
				return -1, nil, fmt.Errorf("error al leer bloque %d: %v", blockPtr, err)
			}

			// Imprimir el contenido del bloque para depuración
			fmt.Println("Contenido del bloque:")
			for entryIndex, entry := range folderBlock.B_content {
				if entry.B_inodo != -1 {
					name := strings.TrimRight(string(entry.B_name[:]), "\x00")
					fmt.Printf("  [%d] %q -> inodo %d\n", entryIndex, name, entry.B_inodo)
				}
			}

			// Buscar el componente actual en el bloque de carpeta
			for _, entry := range folderBlock.B_content {
				if entry.B_inodo == -1 {
					continue
				}

				// Convertir el nombre del archivo a una cadena y eliminar los caracteres nulos
				name := strings.TrimRight(string(entry.B_name[:]), "\x00")
				fmt.Printf("Comparando '%s' con '%s'\n", name, component)

				// Si el nombre del archivo coincide con el componente actual, actualizar el inodo actual
				if name == component {
					currentInodeNum = entry.B_inodo
					found = true
					fmt.Printf("¡Encontrado! El inodo para '%s' es %d\n", component, currentInodeNum)
					break
				}
			}
			// Si se encontró el componente actual, salir del bucle de bloques
			if found {
				break
			}
		}

		// Si no se encontró el componente actual, devolver un error
		if !found {
			return -1, nil, fmt.Errorf("no se encontró '%s' en el directorio actual", component)
		}
	}

	// Leer el inodo final
	targetInode := &Inode{}
	offset := int64(sb.S_inode_start + currentInodeNum*sb.S_inode_size)
	fmt.Printf("Obteniendo inodo final %d en offset %d\n", currentInodeNum, offset)

	// Debugeando
	if err := targetInode.Deserialize(diskPath, offset); err != nil {
		return -1, nil, fmt.Errorf("error al leer inodo final %d: %v", currentInodeNum, err)
	}

	// Debugeando
	fmt.Printf("Inodo encontrado - tipo: %s, tamaño: %d\n",
		string(targetInode.I_type[:]), targetInode.I_size)

	return currentInodeNum, targetInode, nil
}

// ReadFileContent lee el contenido completo de un archivo, manejando indirección.
func ReadFileContent(sb *SuperBlock, diskPath string, inode *Inode) (string, error) {
	// Validaciones iniciales
	if inode == nil {
		return "", errors.New("inodo proporcionado es nil")
	}
	if inode.I_type[0] != '1' {
		// No podemos obtener el índice aquí fácilmente, pero sí el tipo
		return "", fmt.Errorf("el inodo no es de tipo archivo (tipo: %c)", inode.I_type[0])
	}
	if inode.I_size < 0 {
		return "", fmt.Errorf("tamaño de archivo inválido en inodo: %d", inode.I_size)
	}
	if inode.I_size == 0 {
		return "", nil // Archivo vacío, retornar string vacío sin error
	}
	if sb.S_block_size <= 0 {
		return "", errors.New("tamaño de bloque inválido en superbloque")
	}

	fmt.Printf("Leyendo contenido de archivo (tamaño: %d bytes)\n", inode.I_size)

	// Buffer para construir el contenido eficientemente
	var content bytes.Buffer
	content.Grow(int(inode.I_size)) // Pre-asignar capacidad

	// Función auxiliar para leer un bloque de datos y añadirlo al buffer
	readBlock := func(blockPtr int32) error {
		// Verificar si ya leímos todo lo necesario
		if int32(content.Len()) >= inode.I_size {
			fmt.Printf("  readBlock: Límite de tamaño %d alcanzado (%d leídos). Deteniendo.\n", inode.I_size, content.Len())
			return nil // Ya se leyó suficiente, no es un error
		}

		// Validar puntero de bloque
		if blockPtr == -1 {
			// Esto podría indicar un "sparse file" o un error si se esperaba un bloque.
			fmt.Printf("  readBlock: Puntero -1 encontrado. Tratando como bloque vacío/sparse.\n")
			return nil // No leer nada, continuar
		}
		if blockPtr < 0 || blockPtr >= sb.S_blocks_count {
			// Error grave, puntero corrupto
			return fmt.Errorf("puntero de bloque de datos inválido: %d (rango 0-%d)", blockPtr, sb.S_blocks_count-1)
		}

		fmt.Printf("  readBlock: Leyendo bloque de datos %d...\n", blockPtr)
		fileBlock := &FileBlock{} // Usar FileBlock
		blockOffset := int64(sb.S_block_start) + int64(blockPtr)*int64(sb.S_block_size)
		if err := fileBlock.Deserialize(diskPath, blockOffset); err != nil {
			// Error al leer el bloque físico
			return fmt.Errorf("error leyendo bloque de datos %d desde offset %d: %w", blockPtr, blockOffset, err)
		}

		// Calcular cuántos bytes copiar de este bloque
		remainingInFile := inode.I_size - int32(content.Len())
		bytesAvailableInBlock := int32(len(fileBlock.B_content)) // Suele ser 64
		if bytesAvailableInBlock > sb.S_block_size {
			bytesAvailableInBlock = sb.S_block_size
		} // Asegurar no exceder tamaño bloque

		bytesToCopy := bytesAvailableInBlock
		if bytesToCopy > remainingInFile {
			bytesToCopy = remainingInFile // No copiar más allá del tamaño del archivo
		}

		// Copiar los bytes necesarios
		if bytesToCopy > 0 {
			fmt.Printf("    Copiando %d bytes desde bloque %d\n", bytesToCopy, blockPtr)
			// Usar slice con tamaño exacto
			written, errWrite := content.Write(fileBlock.B_content[:bytesToCopy])
			if errWrite != nil {
				// Error escribiendo en el buffer de memoria (muy improbable)
				return fmt.Errorf("error escribiendo en buffer interno: %w", errWrite)
			}
			if int32(written) != bytesToCopy {
				return fmt.Errorf("escritura incompleta en buffer interno (%d vs %d)", written, bytesToCopy)
			}
		} else {
			fmt.Printf("    No se necesitan más bytes desde bloque %d (remaining=%d)\n", blockPtr, remainingInFile)
		}

		return nil
	} // --- Fin readBlock ---

	// --- Procesar bloques ---
	var errRead error
	blocksProcessed := 0

	// Bloques Directos (0-11)
	fmt.Println("Leyendo bloques directos...")
	for i := 0; i < 12; i++ {
		errRead = readBlock(inode.I_block[i])
		if errRead != nil {
			return "", fmt.Errorf("error en bloque directo %d (puntero %d): %w", i, inode.I_block[i], errRead)
		}
		blocksProcessed++
		// Verificar si ya terminamos después de procesar este bloque
		if int32(content.Len()) >= inode.I_size {
			break
		}
	}
	if int32(content.Len()) >= inode.I_size {
		fmt.Println("Contenido completo leído desde bloques directos.")
		return content.String(), nil // Terminado
	}

	// Indirecto Simple (12)
	if inode.I_block[12] != -1 {
		fmt.Printf("Leyendo bloques desde Indirecto Simple (L1 en %d)...\n", inode.I_block[12])
		errRead = readIndirectBlocksRecursive(1, inode.I_block[12], sb, diskPath, &content, inode.I_size, readBlock)
		if errRead != nil {
			return "", fmt.Errorf("error en indirección simple (puntero %d): %w", inode.I_block[12], errRead)
		}
		if int32(content.Len()) >= inode.I_size {
			fmt.Println("Contenido completo leído (alcanzado en indirección simple).")
			return content.String(), nil // Terminado
		}
	}

	// Indirecto Doble (13)
	if inode.I_block[13] != -1 {
		fmt.Printf("Leyendo bloques desde Indirecto Doble (L1 en %d)...\n", inode.I_block[13])
		errRead = readIndirectBlocksRecursive(2, inode.I_block[13], sb, diskPath, &content, inode.I_size, readBlock)
		if errRead != nil {
			return "", fmt.Errorf("error en indirección doble (puntero %d): %w", inode.I_block[13], errRead)
		}
		if int32(content.Len()) >= inode.I_size {
			fmt.Println("Contenido completo leído (alcanzado en indirección doble).")
			return content.String(), nil // Terminado
		}
	}

	// Indirecto Triple (14)
	if inode.I_block[14] != -1 {
		fmt.Printf("Leyendo bloques desde Indirecto Triple (L1 en %d)...\n", inode.I_block[14])
		errRead = readIndirectBlocksRecursive(3, inode.I_block[14], sb, diskPath, &content, inode.I_size, readBlock)
		if errRead != nil {
			return "", fmt.Errorf("error en indirección triple (puntero %d): %w", inode.I_block[14], errRead)
		}
		// No necesitamos verificar tamaño aquí, es la última etapa
	}

	// Verificación final: si después de todo, no se leyó el tamaño esperado, hay un problema
	if int32(content.Len()) < inode.I_size {
		fmt.Printf("Advertencia: Se leyeron %d bytes pero el tamaño del inodo es %d. ¿Punteros faltantes o corruptos?\n", content.Len(), inode.I_size)
		// Podríamos retornar error o el contenido parcial. Retornamos parcial.
	} else if int32(content.Len()) > inode.I_size {
		// Esto no debería pasar con la lógica de readBlock, pero por si acaso. Truncar.
		fmt.Printf("Advertencia: Se leyeron %d bytes pero el tamaño del inodo es %d. Truncando.\n", content.Len(), inode.I_size)
		return content.String()[:inode.I_size], nil
	}

	fmt.Printf("Lectura de archivo completada. Total bytes leídos: %d\n", content.Len())
	return content.String(), nil
}

// readIndirectBlocksRecursive: Función auxiliar recursiva para leer bloques indirectos.
func readIndirectBlocksRecursive(
	level int, // Nivel de indirección actual (1, 2, 3)
	blockPtr int32, // Puntero al bloque de punteros de este nivel
	sb *SuperBlock,
	diskPath string,
	content *bytes.Buffer, // Usar buffer para eficiencia
	sizeLimit int32,
	readBlockFunc func(int32) error, // Función para leer un bloque de DATOS
) error {

	// Condición de parada: nivel inválido o puntero inválido
	// No verificar sizeLimit aquí, se verifica dentro del bucle y en la función llamadora
	if level < 1 || level > 3 {
		return fmt.Errorf("nivel de indirección inválido: %d", level)
	}
	if blockPtr == -1 {
		// fmt.Printf("  readIndirect L%d: Puntero -1, nada que hacer.\n", level)
		return nil // No es un error, simplemente no hay bloque aquí
	}
	if blockPtr < 0 || blockPtr >= sb.S_blocks_count {
		return fmt.Errorf("puntero inválido %d encontrado en indirección nivel %d", blockPtr, level)
	}

	fmt.Printf("  readIndirect L%d: Procesando bloque de punteros %d...\n", level, blockPtr)

	// Deserializar el bloque de punteros de este nivel
	ptrBlock := &PointerBlock{} // Usar PointerBlock
	ptrOffset := int64(sb.S_block_start) + int64(blockPtr)*int64(sb.S_block_size)
	if err := ptrBlock.Deserialize(diskPath, ptrOffset); err != nil {
		// Loguear error pero intentar continuar si es posible? O retornar error?
		// Si no podemos leer el bloque de punteros, no podemos seguir por esta rama.
		return fmt.Errorf("error al leer bloque de punteros nivel %d (índice %d, offset %d): %w", level, blockPtr, ptrOffset, err)
	}

	// Iterar sobre los punteros de este bloque
	for i, nextPtr := range ptrBlock.P_pointers {
		// Detener si ya hemos leído suficiente ANTES de procesar el siguiente puntero
		if int32(content.Len()) >= sizeLimit {
			// fmt.Printf("  readIndirect L%d: Límite de tamaño alcanzado en índice %d. Deteniendo.\n", level, i)
			break // Salir del bucle for P_pointers
		}

		if nextPtr == -1 {
			continue // Puntero no usado
		}
		// Validar el nextPtr antes de usarlo
		if nextPtr < 0 || nextPtr >= sb.S_blocks_count {
			fmt.Printf("Advertencia: puntero inválido %d encontrado en bloque L%d (índice %d, entrada %d). Saltando.\n", nextPtr, level, blockPtr, i)
			continue
		}

		var errRec error
		// Si es el último nivel de indirección (nivel 1), los punteros apuntan a bloques de DATOS
		if level == 1 {
			fmt.Printf("    L1[%d]: Apunta a bloque de datos %d. Llamando a readBlockFunc.\n", i, nextPtr)
			errRec = readBlockFunc(nextPtr) // Llama a la función que lee FileBlock
		} else {
			// Si no es el último nivel, llamar recursivamente para el siguiente nivel inferior
			fmt.Printf("    L%d[%d]: Apunta a bloque L%d en %d. Llamando recursivamente.\n", level, i, level-1, nextPtr)
			errRec = readIndirectBlocksRecursive(level-1, nextPtr, sb, diskPath, content, sizeLimit, readBlockFunc)
		}

		// Manejar error de la llamada recursiva o de readBlockFunc
		if errRec != nil {
			// Propagar el error hacia arriba, añadiendo contexto
			return fmt.Errorf("error procesando puntero %d (L%d[%d] en bloque %d): %w", nextPtr, level, i, blockPtr, errRec)
		}

	} // Fin del bucle for P_pointers

	return nil // Éxito para este nivel
}
