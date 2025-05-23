package structures

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

type FolderBlock struct {
	B_content [4]FolderContent // 4 * 16 = 64 bytes
	// Total: 64 bytes
}

type FolderContent struct {
	B_name  [12]byte
	B_inodo int32
	// Total: 16 bytes
}

// Serialize escribe la estructura FolderBlock en un archivo binario en la posición especificada
func (fb *FolderBlock) Serialize(path string, offset int64) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Mover el puntero del archivo a la posición especificada
	_, err = file.Seek(offset, 0)
	if err != nil {
		return err
	}

	// Serializar la estructura FolderBlock directamente en el archivo
	err = binary.Write(file, binary.LittleEndian, fb)
	if err != nil {
		return err
	}

	return nil
}

// Deserialize lee la estructura FolderBlock desde un archivo binario en la posición especificada
func (fb *FolderBlock) Deserialize(path string, offset int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Mover el puntero del archivo a la posición especificada
	_, err = file.Seek(offset, 0)
	if err != nil {
		return err
	}

	// Obtener el tamaño de la estructura FolderBlock
	fbSize := binary.Size(fb)
	if fbSize <= 0 {
		return fmt.Errorf("invalid FolderBlock size: %d", fbSize)
	}

	// Leer solo la cantidad de bytes que corresponden al tamaño de la estructura FolderBlock
	buffer := make([]byte, fbSize)
	bytesRead, err := file.Read(buffer)
	if err != nil {
		return err
	}

	if bytesRead < fbSize {
        return fmt.Errorf("no se pudieron leer todos los bytes: leídos %d, esperados %d", bytesRead, fbSize)
    }

	// Deserializar los bytes leídos en la estructura FolderBlock
	reader := bytes.NewReader(buffer)
	err = binary.Read(reader, binary.LittleEndian, fb)
	if err != nil {
		return err
	}

	return nil
}

// Print imprime el contenido del bloque de carpeta
func (fb *FolderBlock) Print() {
	fmt.Println("  Contenido del FolderBlock:")
	for i, content := range fb.B_content {
		// Solo imprimir entradas válidas (inodo != -1)
		if content.B_inodo != -1 {
			name := strings.TrimRight(string(content.B_name[:]), "\x00") // Limpiar nulos para mostrar
			fmt.Printf("    [%d] Nombre: %-12q Inodo: %d\n", i, name, content.B_inodo)
		} else {
			fmt.Printf("    [%d] (Slot Libre)\n", i)
		}
	}
}


func (fb *FolderBlock) Initialize() {
	for i := range fb.B_content {
		fb.B_content[i].B_inodo = -1 //lot libre
		fb.B_content[i].B_name = [12]byte{}
	}
}
