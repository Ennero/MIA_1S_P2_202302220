package analyzer

import (
	commands "backend/commands"
	"fmt"    
	"strings" 
)

func Analyzer(input string) (string, error) {

	trimmedInput := strings.TrimSpace(input)

	//Ignorar líneas vacías o que son solo comentarios
	if trimmedInput == "" {
		// Línea vacía o solo espacios en blanco, no hacer nada, no es un error.
		return "", nil
	}
	if strings.HasPrefix(trimmedInput, "#") {
        fmt.Printf("Comentario ignorado: %s\n", trimmedInput)
		return "", nil
	}

	//Dividir la línea en tokens
	tokens := strings.Fields(trimmedInput)

	if len(tokens) == 0 {
		return "", nil 
	}

	// Procesar el comando 
	command := strings.ToLower(tokens[0]) // Convertir comando a minúsculas
	arguments := tokens[1:]  

	// Switch para manejar comandos conocidos
	switch command {
	case "mkdisk":
		return commands.ParseMkdisk(arguments)
	case "fdisk":
		return commands.ParseFdisk(arguments)
	case "mount":
		return commands.ParseMount(arguments)
	case "mkfs":
		return commands.ParseMkfs(arguments)
	case "rep":
		return commands.ParseRep(arguments)
	case "mkdir":
		return commands.ParseMkdir(arguments)
	case "rmdisk":
		return commands.ParseRmdisk(arguments)
	case "mounted":
		return commands.ParseMounted(arguments)
	case "cat":
		return commands.ParseCat(arguments)
	case "login":
		return commands.ParseLogin(arguments)
	case "logout":
		return commands.ParseLogout(arguments)
	case "mkfile":
		return commands.ParseMkfile(arguments)
	case "mkgrp":
		return commands.ParseMkgrp(arguments)
	case "rmgrp":
		return commands.ParseRmgrp(arguments)
	case "mkusr":
		return commands.ParseMkusr(arguments)
	case "rmusr":
		return commands.ParseRmusr(arguments)
	case "chgrp":
		return commands.ParseChgrp(arguments)

	default:

		return "", fmt.Errorf("comando desconocido: %s", command)
	}
}