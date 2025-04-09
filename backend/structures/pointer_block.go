package structures

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

type PointerBlock struct {
	P_pointers [16]int32 // 16 * 4 = 64 bytes
	// Total: 64 bytes
}



func (pb *PointerBlock) Deserialize(path string, offset int64) error {
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

	// Obtener el tamaño de la estructura PointerBlock
	pbSize := binary.Size(pb)
	if pbSize <= 0 {
		return fmt.Errorf("invalid PointerBlock size: %d", pbSize)
	}
	
	// Leer solo la cantidad de bytes que corresponden al tamaño de la estructura PointerBlock
	buffer := make([]byte, pbSize)
	_, err = file.Read(buffer)
	if err != nil {
		return err
	}

	// Deserializar los bytes leídos en la estructura PointerBlock
	reader := bytes.NewReader(buffer)
	err = binary.Read(reader, binary.LittleEndian, pb)
	if err != nil {
		return err
	}

	return nil
}

// Serialize escribe la estructura FileBlock en un archivo binario en la posición especificada
func (pb *PointerBlock) Serialize(path string, offset int64) error {
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

	// Serializar la estructura FileBlock directamente en el archivo
	err = binary.Write(file, binary.LittleEndian, pb)
	if err != nil {
		return err
	}

	return nil
}


