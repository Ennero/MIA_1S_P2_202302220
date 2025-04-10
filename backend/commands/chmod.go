package commands



type CHMOD struct {
	path string
	UGO  [3]string
}



func ParseChmod(tokens []string) (string, error) {
	return "", nil	
}