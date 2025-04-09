package commands

import (
	reports "backend/reports"
	stores "backend/stores"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// REP estructura que representa el comando rep con sus parámetros
type REP struct {
	id           string // ID del disco
	path         string // Ruta del archivo del disco
	name         string // Nombre del reporte
	path_file_ls string // Ruta del archivo ls (opcional)
}

func ParseRep(tokens []string) (string, error) {
	cmd := &REP{}

	args := strings.Join(tokens, " ")
	fmt.Printf("Argumentos REP completos: %s\n", args)

	// Regex más simple y robusta si asumimos que los valores sin comillas no tienen espacios:
	re := regexp.MustCompile(`-(?i)(id|name|path|path_file_ls)=("[^"]+"|[^\s]+)`)

	matches := re.FindAllStringSubmatch(args, -1) // Usar Submatch para capturar grupos
	fmt.Printf("Coincidencias REP encontradas: %v\n", matches)

	processedKeys := make(map[string]bool) // Para evitar parámetros duplicados

	for _, match := range matches {
		if len(match) < 3 {
			continue
		} // Asegurarse que hay suficientes grupos capturados

		key := strings.ToLower(match[1]) // Obtener el nombre del parámetro (ya en minúsculas por (?i) pero ToLower asegura)
		value := match[2]                // Obtener el valor capturado (puede tener comillas)

		fmt.Printf("  Procesando: key='%s', raw_value='%s'\n", key, value)

		// Verificar si la clave ya fue procesada
		if processedKeys[key] {
			return "", fmt.Errorf("parámetro duplicado: -%s", key)
		}

		// Quitar comillas dobles iniciales Y/O finales si existen
		originalValueForDebug := value // Opcional: guardar para comparar en debug
		value = strings.Trim(value, "\"")
		if value != originalValueForDebug {
			fmt.Printf("    Valor sin comillas: '%s'\n", value)
		} else {
			fmt.Printf("    Valor sin comillas (sin cambios): '%s'\n", value)
		}

		// Validar valor vacío después de quitar comillas (excepto para flags si los hubiera)
		if value == "" && key != "algun_flag_sin_valor" { // Añadir flags aquí si existen
			return "", fmt.Errorf("el valor para el parámetro -%s no puede estar vacío", key)
		}
		switch key {
		case "id":
			cmd.id = value
			processedKeys["id"] = true
		case "path":
			cmd.path = value
			processedKeys["path"] = true
		case "name":
			nameLower := strings.ToLower(value) // Convertir valor a minúsculas para comparación
			validNames := []string{"mbr", "disk", "inode", "block", "bm_inode", "bm_block", "sb", "file", "ls", "tree"}
			if !slices.Contains(validNames, nameLower) {
				return "", fmt.Errorf("valor inválido para -name: '%s'. Debe ser uno de: %s", value, strings.Join(validNames, ", "))
			}
			cmd.name = nameLower
			processedKeys["name"] = true
		case "path_file_ls":
			cmd.path_file_ls = value
			processedKeys["path_file_ls"] = true
		default:
			return "", fmt.Errorf("parámetro desconocido detectado internamente: %s", key)
		}
	}

	// Verifica que los parámetros obligatorios hayan sido proporcionados
	if !processedKeys["id"] {
		return "", errors.New("parámetro requerido faltante: -id")
	}
	if !processedKeys["path"] {
		return "", errors.New("parámetro requerido faltante: -path")
	}
	if !processedKeys["name"] {
		return "", errors.New("parámetro requerido faltante: -name")
	}

	if (cmd.name == "file" || cmd.name == "ls") && !processedKeys["path_file_ls"] {
		return "", fmt.Errorf("el parámetro -path_file_ls es requerido para el reporte '%s'", cmd.name)
	}

	err := commandRep(cmd)
	if err != nil {
		return "", err
	}

	successMsg := fmt.Sprintf("REP: Reporte generado exitosamente\n"+
		"-> ID: %s\n"+
		"-> Path: %s\n"+
		"-> Tipo: %s",
		cmd.id,
		cmd.path,
		cmd.name,
	)
	if cmd.path_file_ls != "" {
		successMsg += fmt.Sprintf("\n-> Path LS/File: %s", cmd.path_file_ls)
	}
	return successMsg, nil
}

func commandRep(rep *REP) error {
	// Obtener la partición montada
	mountedMbr, mountedSb, mountedDiskPath, err := stores.GetMountedPartitionRep(rep.id)
	if err != nil {
		return err
	}

	// Switch para manejar diferentes tipos de reportes
	switch rep.name {
	case "mbr":
		err = reports.ReportMBR(mountedMbr, mountedDiskPath, rep.path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}
	case "inode":
		err = reports.ReportInode(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err

		}
	case "bm_inode":
		err = reports.ReportBMInode(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err

		}
	case "disk":
		err = reports.ReportDisk(mountedMbr, mountedDiskPath, rep.path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err

		}
	case "bm_block":
		err = reports.ReportBMBlock(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err

		}
	case "sb":
		err = reports.ReportSuperBlock(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err

		}
	case "block":
		err = reports.ReportBlock(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}
	case "tree":
		err = reports.ReportTree(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}

	case "file":
		err = reports.ReportFile(mountedSb, mountedDiskPath, rep.path, rep.path_file_ls)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}
	case "ls":
		err = reports.ReportLS(mountedSb, mountedDiskPath, rep.path, rep.path_file_ls)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}

	}
	return nil
}
