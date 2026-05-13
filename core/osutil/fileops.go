package osutil

import "os"

// ReadFileString reads a whole file as a string.
func ReadFileString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ReadFileBytes reads a whole file as bytes.
func ReadFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFileString writes or appends string content to a file.
func WriteFileString(path, content string, appendFile bool) error {
	flags := os.O_CREATE | os.O_WRONLY
	if appendFile {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
