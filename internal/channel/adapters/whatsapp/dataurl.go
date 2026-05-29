package whatsapp

import (
	"encoding/base64"
	"errors"
	"strings"
)

// base64Encode is a thin wrapper to keep the call site readable.
func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// decodeDataURL parses a data: URL and returns the raw bytes plus the declared
// MIME type. Only base64-encoded payloads are supported (the form produced by
// encodeDataURL).
func decodeDataURL(raw string) ([]byte, string, error) {
	value := strings.TrimSpace(raw)
	if !strings.HasPrefix(value, "data:") {
		return nil, "", errors.New("not a data URL")
	}
	value = strings.TrimPrefix(value, "data:")
	comma := strings.Index(value, ",")
	if comma < 0 {
		return nil, "", errors.New("malformed data URL")
	}
	header := value[:comma]
	payload := value[comma+1:]
	parts := strings.Split(header, ";")
	mime := strings.TrimSpace(parts[0])
	isBase64 := false
	for _, p := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(p), "base64") {
			isBase64 = true
		}
	}
	if !isBase64 {
		return nil, "", errors.New("data URL is not base64 encoded")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, mime, err
	}
	return data, mime, nil
}
