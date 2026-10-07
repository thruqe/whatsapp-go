package sdk

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// EncodeBase64 encodes bytes into standard RFC 4648 base64.
func EncodeBase64(bytes []byte) string {
	return base64.StdEncoding.EncodeToString(bytes)
}

// DecodeBase64 decodes a base64 string, ignoring any whitespace characters.
func DecodeBase64(input string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, input)
	return base64.StdEncoding.DecodeString(cleaned)
}

// ToDataURL formats base64 payload into a Data URL (data:<mimetype>;base64,<data>).
func ToDataURL(mimetype, base64Data string) string {
	return fmt.Sprintf("data:%s;base64,%s", mimetype, base64Data)
}

// ReadFileAsBase64 reads a local file and returns its content as a Base64 string.
func ReadFileAsBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return EncodeBase64(data), nil
}

// ReadFileAsDataURL reads a local file and formats it as a Data URL.
func ReadFileAsDataURL(path, mimetype string) (string, error) {
	b64, err := ReadFileAsBase64(path)
	if err != nil {
		return "", err
	}
	return ToDataURL(mimetype, b64), nil
}
