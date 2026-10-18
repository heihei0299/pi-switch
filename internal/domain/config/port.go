package config

import "fmt"

// ValidatePort runs before narrowing a user-supplied port to uint16.
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d must be between 1 and 65535", port)
	}
	return nil
}
