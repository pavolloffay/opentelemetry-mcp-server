package utils

import (
	"os"
	"strconv"
)

func GetEnvAsInt(envKey string, fallback int) int {
	if valStr := os.Getenv(envKey); valStr != "" {
		if val, err := strconv.Atoi(valStr); err == nil {
			return val
		}
	}
	return fallback
}
