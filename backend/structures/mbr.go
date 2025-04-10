package structures

import (
	"bytes"           // Paquete para manipulación de buffers
	"encoding/binary" // Paquete para codificación y decodificación de datos binarios
	"errors"
	"fmt"  // Paquete para formateo de E/S
	"math" // Para math.MaxInt32 en Best Fit
	"os"   // Paquete para funciones del sistema operativo
	"sort" // Para ordenar particiones
	"strings"
	"time"
)

type MBR struct {
	Mbr_size           int32        // Tamaño del MBR en bytes
	Mbr_creation_date  float32      // Fecha y hora de creación del MBR
	Mbr_disk_signature int32        // Firma del disco
	Mbr_disk_fit       [1]byte      // Tipo de ajuste
	Mbr_partitions     [4]Partition // Particiones del MBR
}

// SerializeMBR escribe la estructura MBR al inicio de un archivo binario
func (mbr *MBR) Serialize(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Serializar la estructura MBR directamente en el archivo
	err = binary.Write(file, binary.LittleEndian, mbr)
	if err != nil {
		return err
	}

	return nil
}

// DeserializeMBR lee la estructura MBR desde el inicio de un archivo binario
func (mbr *MBR) Deserialize(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Obtener el tamaño de la estructura MBR
	mbrSize := binary.Size(mbr)
	if mbrSize <= 0 {
		return fmt.Errorf("invalid MBR size: %d", mbrSize)
	}

	// Leer solo la cantidad de bytes que corresponden al tamaño de la estructura MBR
	buffer := make([]byte, mbrSize)
	_, err = file.Read(buffer)
	if err != nil {
		return err
	}

	// Deserializar los bytes leídos en la estructura MBR
	reader := bytes.NewReader(buffer)
	err = binary.Read(reader, binary.LittleEndian, mbr)
	if err != nil {
		return err
	}

	return nil
}

// Método para obtener una lista de los nombres de las particiones
func (mbr *MBR) GetPartitionNames() []string {
	// Crear una lista de nombres de particiones
	partitionNames := make([]string, 0)

	// Recorrer las particiones del MBR
	for _, partition := range mbr.Mbr_partitions {
		// Convertir Part_name a string y eliminar los caracteres nulos
		partitionName := strings.Trim(string(partition.Part_name[:]), "\x00 ")
		// Agregar el nombre de la partición a la lista
		partitionNames = append(partitionNames, partitionName)
	}
	return partitionNames
}

func (mbr *MBR) GetPartitionByName(name string) (*Partition, int, error) {
	inputName := strings.Trim(name, "\x00 ") // Limpiar nombre buscado
	for i := range mbr.Mbr_partitions {
		// Considerar solo particiones existentes
		if mbr.Mbr_partitions[i].Part_status != [1]byte{0} && mbr.Mbr_partitions[i].Part_status[0] != 'N' && mbr.Mbr_partitions[i].Part_size > 0 {
			partitionName := strings.TrimRight(string(mbr.Mbr_partitions[i].Part_name[:]), "\x00 ")
			if strings.EqualFold(partitionName, inputName) {
				return &mbr.Mbr_partitions[i], i, nil
			}
		}
	}
	// Si no se encontró, devolver nil, -1 y un error
	return nil, -1, fmt.Errorf("partición primaria/extendida con nombre '%s' no encontrada", name)
}

// Función para obtener una partición por ID
func (mbr *MBR) GetPartitionByID(id string) (*Partition, error) {
	for i := 0; i < len(mbr.Mbr_partitions); i++ {
		// Convertir Part_name a string y eliminar los caracteres nulos
		partitionID := strings.Trim(string(mbr.Mbr_partitions[i].Part_id[:]), "\x00 ")
		// Convertir el id a string y eliminar los caracteres nulos
		inputID := strings.Trim(id, "\x00 ")
		// Si el nombre de la partición coincide, devolver la partición
		if strings.EqualFold(partitionID, inputID) {
			return &mbr.Mbr_partitions[i], nil
		}
	}
	return nil, errors.New("partición no encontrada")
}

// Método para imprimir los valores del MBR
func (mbr *MBR) PrintMBR() {
	// Convertir Mbr_creation_date a time.Time
	creationTime := time.Unix(int64(mbr.Mbr_creation_date), 0)

	// Convertir Mbr_disk_fit a char
	diskFit := rune(mbr.Mbr_disk_fit[0])

	fmt.Printf("MBR Size: %d\n", mbr.Mbr_size)
	fmt.Printf("Creation Date: %s\n", creationTime.Format(time.RFC3339))
	fmt.Printf("Disk Signature: %d\n", mbr.Mbr_disk_signature)
	fmt.Printf("Disk Fit: %c\n", diskFit)
}

// Método para imprimir las particiones del MBR
func (mbr *MBR) PrintPartitions() {
	for i, partition := range mbr.Mbr_partitions {
		// Convertir Part_status, Part_type y Part_fit a char
		partStatus := rune(partition.Part_status[0])
		partType := rune(partition.Part_type[0])
		partFit := rune(partition.Part_fit[0])

		// Convertir Part_name a string
		partName := string(partition.Part_name[:])
		// Convertir Part_id a string
		partID := string(partition.Part_id[:])

		fmt.Printf("Partition %d:\n", i+1)
		fmt.Printf("  Status: %c\n", partStatus)
		fmt.Printf("  Type: %c\n", partType)
		fmt.Printf("  Fit: %c\n", partFit)
		fmt.Printf("  Start: %d\n", partition.Part_start)
		fmt.Printf("  Size: %d\n", partition.Part_size)
		fmt.Printf("  Name: %s\n", partName)
		fmt.Printf("  Correlative: %d\n", partition.Part_correlative)
		fmt.Printf("  ID: %s\n", partID)
	}
}

//Mejora de firsAvailablePartition--------------------------------------------
// Gap representa un hueco de espacio libre en el disco
type Gap struct {
	Start int32
	Size  int32
}

// Busca un hueco adecuado y un slot MBR libre para una nueva partición.
func (mbr *MBR) GetFirstAvailablePartition(requestedSize int32, fit byte) (*Partition, int32, int, error) {
	if requestedSize <= 0 {
		return nil, 0, -1, errors.New("tamaño solicitado debe ser positivo")
	}

	fmt.Printf("Buscando espacio para %d bytes con ajuste %c\n", requestedSize, fit)

	// Crear lista de particiones existentes y ordenarlas
	existingPartitions := []Partition{}
	unusedSlotIndices := []int{}
	for i := 0; i < 4; i++ {
		// Usar Part_size > 0 como indicador principal de "ocupado"
		if mbr.Mbr_partitions[i].Part_size > 0 {
			existingPartitions = append(existingPartitions, mbr.Mbr_partitions[i])
		} else {
			unusedSlotIndices = append(unusedSlotIndices, i)
		}
	}

	// Verificar si quedan slots libres en el MBR
	if len(unusedSlotIndices) == 0 {
		return nil, 0, -1, errors.New("no hay slots libres en la tabla de particiones MBR")
	}
	firstUnusedSlotIndex := unusedSlotIndices[0] // El primer slot libre encontrado

	fmt.Printf("Slots MBR libres encontrados: %v (usando índice %d si se encuentra hueco)\n", unusedSlotIndices, firstUnusedSlotIndex)

	sort.Slice(existingPartitions, func(i, j int) bool {
		return existingPartitions[i].Part_start < existingPartitions[j].Part_start
	})

	// Calcular Huecos
	gaps := []Gap{}
	mbrStructSize := int32(binary.Size(MBR{})) // Tamaño de la estructura MBR en sí
	lastEndOffset := mbrStructSize             // Empezar a buscar espacio DESPUÉS del MBR

	for _, part := range existingPartitions {
		// Validar consistencia básica
		if part.Part_start < lastEndOffset {
			fmt.Printf("Advertencia: ¡Solapamiento detectado! Partición '%s' empieza en %d antes del final anterior %d.\n", strings.TrimRight(string(part.Part_name[:]), "\x00"), part.Part_start, lastEndOffset)
			// Ajustar lastEndOffset para evitar errores, pero indica MBR corrupto
			lastEndOffset = part.Part_start
		}

		currentGapSize := part.Part_start - lastEndOffset
		if currentGapSize >= requestedSize {
			fmt.Printf("  Hueco encontrado: start=%d, size=%d\n", lastEndOffset, currentGapSize)
			gaps = append(gaps, Gap{Start: lastEndOffset, Size: currentGapSize})
		}
		lastEndOffset = part.Part_start + part.Part_size // Actualizar para el siguiente hueco
	}

	// Calcular hueco final 
	finalGapSize := mbr.Mbr_size - lastEndOffset
	if finalGapSize >= requestedSize {
		fmt.Printf("  Hueco final encontrado: start=%d, size=%d\n", lastEndOffset, finalGapSize)
		gaps = append(gaps, Gap{Start: lastEndOffset, Size: finalGapSize})
	}

	if len(gaps) == 0 {
		fmt.Println("No se encontraron huecos suficientemente grandes.")
		return nil, 0, -1, errors.New("no se encontró espacio libre contiguo suficiente para el tamaño solicitado")
	}

	// Aplicar lógica de ajuste (Fit)
	var bestGap Gap
	foundFit := false

	switch fit {
	case 'F': // First Fit
		bestGap = gaps[0] // Tomar el primer hueco encontrado que es >= requestedSize
		foundFit = true
		fmt.Printf("Fit 'FF': Usando primer hueco encontrado (start=%d, size=%d)\n", bestGap.Start, bestGap.Size)

	case 'B': // Best Fit
		minDiff := int32(math.MaxInt32)
		for _, gap := range gaps {
			diff := gap.Size - requestedSize
			if diff >= 0 && diff < minDiff { // Encontrar la menor diferencia positiva o cero
				minDiff = diff
				bestGap = gap
				foundFit = true
			}
		}
		if foundFit {
			fmt.Printf("Fit 'BF': Usando el hueco con menor desperdicio (start=%d, size=%d, diff=%d)\n", bestGap.Start, bestGap.Size, minDiff)
		}

	case 'W': // Worst Fit
		maxSize := int32(-1)
		for _, gap := range gaps {
			if gap.Size >= maxSize { // Encontrar el hueco más grande
				maxSize = gap.Size
				bestGap = gap
				foundFit = true
			}
		}
		if foundFit {
			fmt.Printf("Fit 'WF': Usando el hueco más grande (start=%d, size=%d)\n", bestGap.Start, bestGap.Size)
		}

	default:
		return nil, 0, -1, fmt.Errorf("tipo de ajuste desconocido: %c", fit)
	}

	// Si después del ajuste no hay un hueco Best Fit no encontró ninguno
	if !foundFit {
		return nil, 0, -1, errors.New("no se encontró un hueco adecuado según la estrategia de ajuste")
	}

	// Devolvemos un puntero al slot MBR que se usará y el offset donde debe empezar
	return &mbr.Mbr_partitions[firstUnusedSlotIndex], bestGap.Start, firstUnusedSlotIndex, nil
}

// Código nuevo ---------------------------------------------------------------------------------

// Verifica si un nombre ya está en uso en MBR o lógicas
func (mbr *MBR) IsPartitionNameTaken(name string, diskPath string) bool {
	cleanName := strings.Trim(name, "\x00 ")
	// Verificar MBR
	for i := range mbr.Mbr_partitions {
		if mbr.Mbr_partitions[i].Part_status != [1]byte{0} && mbr.Mbr_partitions[i].Part_status[0] != 'N' && mbr.Mbr_partitions[i].Part_size > 0 {
			partitionName := strings.TrimRight(string(mbr.Mbr_partitions[i].Part_name[:]), "\x00 ")
			if strings.EqualFold(partitionName, cleanName) {
				return true // Encontrado en MBR
			}
		}
	}

	// Verificar Lógicas (si hay extendida)
	extendedPartition, _ := mbr.GetExtendedPartition()
	if extendedPartition != nil {
		file, err := os.Open(diskPath)
		if err != nil {
			fmt.Printf("Advertencia: No se pudo abrir disco para verificar nombres lógicos: %v\n", err)
			return false // No podemos confirmar, asumir que no está tomado
		}
		defer file.Close()

		currentPos := int64(extendedPartition.Part_start)
		for {
			_, err = file.Seek(currentPos, 0)
			if err != nil {
				break
			}
			var currentEBR EBR
			err = binary.Read(file, binary.LittleEndian, &currentEBR)
			if err != nil {
				break
			}

			// Comprobar si el EBR es válido y comparar nombre
			if currentEBR.Part_status[0] != 'N' && currentEBR.Part_size > 0 { // Part_size > 0 para lógicas reales
				ebrName := strings.TrimRight(string(currentEBR.Part_name[:]), "\x00 ")
				if strings.EqualFold(ebrName, cleanName) {
					return true // Encontrado en Lógica
				}
			}

			// Avanzar al siguiente EBR
			if currentEBR.Part_next == -1 {
				break
			}
			if int64(currentEBR.Part_next) <= currentPos {
				fmt.Printf("Advertencia: Ciclo o error en cadena EBR detectado al verificar nombre.\n")
				break // Evitar bucle infinito
			}
			currentPos = int64(currentEBR.Part_next)

			// No salir de la extendida
			if currentPos < int64(extendedPartition.Part_start) || currentPos >= int64(extendedPartition.Part_start+extendedPartition.Part_size) {
				fmt.Printf("Advertencia: Puntero EBR Part_next (%d) fuera de límites de extendida.\n", currentPos)
				break
			}

		}
	}

	return false // No encontrado
}

// Devuelve un puntero a la partición extendida y su índice, o nil si no existe.
func (mbr *MBR) GetExtendedPartition() (*Partition, int) {
	for i := range mbr.Mbr_partitions {
		if mbr.Mbr_partitions[i].Part_status != [1]byte{0} && mbr.Mbr_partitions[i].Part_status[0] != 'N' && mbr.Mbr_partitions[i].Part_type[0] == 'E' {
			return &mbr.Mbr_partitions[i], i
		}
	}
	return nil, -1
}

func (p *Partition) DeletePartition() { // Método para eliminar una partición
	p.Part_status = [1]byte{'N'}
	p.Part_type = [1]byte{' '}
	p.Part_fit = [1]byte{' '}
	p.Part_start = -1
	p.Part_size = 0
	p.Part_name = [16]byte{}
	p.Part_correlative = 0
	p.Part_id = [4]byte{}
}

func (ebr *EBR) Initialize() {
	ebr.Part_status = [1]byte{'N'} // 'N' para No Usado/Libre
	ebr.Part_fit = [1]byte{'W'} 
	ebr.Part_start = -1
	ebr.Part_size = 0
	ebr.Part_next = -1
	ebr.Part_name = [16]byte{}
}

func (p *Partition) PrintPartition() {
	nameStr := strings.TrimRight(string(p.Part_name[:]), "\x00 ")
	idStr := strings.TrimRight(string(p.Part_id[:]), "\x00 ")
	status := p.Part_status[0]
	if status == 0 {
		status = 'N'
	} 
	typeP := p.Part_type[0]
	if typeP == 0 {
		typeP = ' '
	}
	fit := p.Part_fit[0]
	if fit == 0 {
		fit = ' '
	}

	fmt.Printf("    Status: %c | Type: %c | Fit: %c | Start: %-10d | Size: %-10d | Corr: %-2d | ID: %-4s | Name: %s\n",
		status, typeP, fit, p.Part_start, p.Part_size, p.Part_correlative, idStr, nameStr)
}
