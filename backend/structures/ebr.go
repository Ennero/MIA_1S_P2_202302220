package structures

type EBR struct {
	Part_status  [1]byte  // Estado de la partición
	Part_fit     [1]byte  // Tipo de ajuste
	Part_start   int32    // Byte de inicio de la partición
	Part_size    int32    // Tamaño de la partición
	Part_next    int32    // Dirección del siguiente EBR (-1 si no hay otro)
	Part_name    [16]byte // Nombre de la partición
}
