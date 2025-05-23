package stores

import (
	structures "backend/structures"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Carnet de estudiante
const Carnet string = "20" // 202302220

// --- Variables Globales ---
var (
	// Mapa para particiones montadas
	MountedPartitions map[string]string = make(map[string]string)

	// Mapa para discos creados
	DiskRegistry map[string]string = make(map[string]string) 

	ListPatitions []string = make([]string, 0) // Guarda NOMBRES de particiones montadas
	ListMounted   []string = make([]string, 0) // Guarda IDs de particiones montadas
)

// GetMountedPartition obtiene la partición montada con el id especificado
func GetMountedPartition(id string) (*structures.Partition, string, error) {
	// Obtener el path de la partición montada
	path := MountedPartitions[id]
	if path == "" {
		return nil, "", errors.New("la partición no está montada")
	}

	// Crear una instancia de MBR
	var mbr structures.MBR

	// Deserializar la estructura MBR desde un archivo binario
	err := mbr.Deserialize(path)
	if err != nil {
		return nil, "", err
	}

	// Buscar la partición con el id especificado
	partition, err := mbr.GetPartitionByID(id)
	if partition == nil {
		return nil, "", err
	}

	return partition, path, nil
}

// GetMountedMBR obtiene el MBR de la partición montada con el id especificado
func GetMountedPartitionRep(id string) (*structures.MBR, *structures.SuperBlock, string, error) {
	// Obtener el path de la partición montada
	path := MountedPartitions[id]
	if path == "" {
		return nil, nil, "", errors.New("la partición no está montada")
	}

	// Crear una instancia de MBR
	var mbr structures.MBR

	// Deserializar la estructura MBR desde un archivo binario
	err := mbr.Deserialize(path)
	if err != nil {
		return nil, nil, "", err
	}

	// Buscar la partición con el id especificado
	partition, err := mbr.GetPartitionByID(id)
	if partition == nil {
		return nil, nil, "", err
	}

	// Crear una instancia de SuperBlock
	var sb structures.SuperBlock

	// Deserializar la estructura SuperBlock desde un archivo binario
	err = sb.Deserialize(path, int64(partition.Part_start))
	if err != nil {
		return nil, nil, "", err
	}

	return &mbr, &sb, path, nil
}

// GetMountedPartitionSuperblock obtiene el SuperBlock de la partición montada con el id especificado
func GetMountedPartitionSuperblock(id string) (*structures.SuperBlock, *structures.Partition, string, error) {
	// Obtener el path de la partición montada
	path := MountedPartitions[id]
	if path == "" {
		return nil, nil, "", errors.New("la partición no está montada")
	}

	// Crear una instancia de MBR
	var mbr structures.MBR

	// Deserializar la estructura MBR desde un archivo binario
	err := mbr.Deserialize(path)
	if err != nil {
		return nil, nil, "", err
	}

	// Buscar la partición con el id especificado
	partition, err := mbr.GetPartitionByID(id)
	if partition == nil {
		return nil, nil, "", err
	}

	// Crear una instancia de SuperBlock
	var sb structures.SuperBlock

	// Deserializar la estructura SuperBlock desde un archivo binario
	err = sb.Deserialize(path, int64(partition.Part_start))
	if err != nil {
		return nil, nil, "", err
	}

	return &sb, partition, path, nil
}

// PARTE PARA LA AUTENTICACION CON EL LOGIN
// AuthStore almacena la información de autenticación del usuario
type AuthStore struct {
	IsLoggedIn  bool
	Username    string
	Password    string
	PartitionID string
}

var Auth = &AuthStore{
	IsLoggedIn:  false,
	Username:    "",
	Password:    "",
	PartitionID: "",
}

func (a *AuthStore) Login(username, password, partitionID string) {
	a.IsLoggedIn = true
	a.Username = username
	a.Password = password
	a.PartitionID = partitionID
}

func (a *AuthStore) Logout() {
	a.IsLoggedIn = false
	a.Username = ""
	a.Password = ""
	a.PartitionID = ""
}

func (a *AuthStore) IsAuthenticated() bool {
	return a.IsLoggedIn
}

func (a *AuthStore) GetCurrentUser() (string, string, string) {
	return a.Username, a.Password, a.PartitionID
}

func (a *AuthStore) GetPartitionID() string {
	return a.PartitionID
}

func GetMountedPartitionInfo(id string) (*structures.MBR, *structures.Partition, string, error) {
	path := MountedPartitions[id]
	if path == "" {
		return nil, nil, "", fmt.Errorf("partición con id '%s' no está montada", id)
	}

	var mbr structures.MBR

	err := mbr.Deserialize(path)
	if err != nil {
		return &mbr, nil, path, fmt.Errorf("error al leer MBR del disco '%s': %w", path, err)
	}

	// Buscar la partición DENTRO DEL MBR que acabamos de leer
	partition, err := mbr.GetPartitionByID(id) 
	if err != nil {
		return &mbr, nil, path, fmt.Errorf("no se encontró la partición con id '%s' en el MBR del disco '%s': %w", id, path, err)
	}

	return &mbr, partition, path, nil
}

// PARA QUE FUNCIONA LA COSA DEL EXPLORADOR
func GetMountIDForPartition(checkDiskPath string, checkPartName string) (string, bool) {
    cleanCheckDiskPath := filepath.Clean(checkDiskPath)
    cleanCheckPartName := strings.TrimSpace(checkPartName)

    for mountID, mountedDiskPath := range MountedPartitions {
        if filepath.Clean(mountedDiskPath) == cleanCheckDiskPath {
            var mbr structures.MBR
            err := mbr.Deserialize(mountedDiskPath) 
            if err != nil {
                fmt.Printf("Advertencia: No se pudo leer MBR de '%s' al buscar ID %s\n", mountedDiskPath, mountID)
                continue // Saltar este ID montado
            }
            part, errPart := mbr.GetPartitionByID(mountID) // Buscar por ID de montaje
            if errPart == nil && part != nil {
                nameInMBR := strings.TrimRight(string(part.Part_name[:]), "\x00 ")
                // Comprobar si el nombre en el MBR coincide con el que buscamos
                if strings.EqualFold(nameInMBR, cleanCheckPartName) {
                    return mountID, true 
                }
            }
        }
    }
    return "", false
}