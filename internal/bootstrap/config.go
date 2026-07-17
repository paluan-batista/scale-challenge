// Package bootstrap contains process-level configuration checks used before any
// network client or background processing is started.
package bootstrap

import (
	"fmt"
	"os"
	"strings"
)

// RequiredEnvironment returns a redacted configuration error for missing names.
// It intentionally reports only variable names, never their values.
func RequiredEnvironment(names ...string) error {
	for _, name := range names {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			return fmt.Errorf("required environment variable %s is not set", name)
		}
	}
	return nil
}
