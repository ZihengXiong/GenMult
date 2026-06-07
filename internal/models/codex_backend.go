package models

import "strings"

func useNativeCodexBackend(apiKey, codexAccountID string) bool {
	if strings.TrimSpace(codexAccountID) != "" {
		return true
	}
	parts := strings.Split(strings.TrimSpace(apiKey), ".")
	return len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != ""
}
