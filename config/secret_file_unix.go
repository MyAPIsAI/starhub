//go:build !windows

package config

import "os"

func protectSecretFile(path string) error { return os.Chmod(path, 0600) }
