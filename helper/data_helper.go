package helper

import (
	"errors"
	"strings"
)

func ParseInfo(line string) (string, string, error) {
	parts := strings.Split(line, ":")
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}
	return "", "", errors.New("invalid format")
}
