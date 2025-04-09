package stores

import (
	structures "backend/structures"
	"errors"
)

// Carnet de estudiante
const Carnet string = "20" // 202302220

// Declaración de variables globales
var (
	MountedPartitions map[string]string = make(map[string]string)
)

//Lista para saber si ya se montó alguna particion
var ListPatitions []string = make([]string, 0)

//Esta lista es para el mounted xd
var ListMounted []string = make([]string, 0)


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













