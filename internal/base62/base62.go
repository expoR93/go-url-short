package base62

import (
	"errors"
	"strings"
)

// The character set for Base62
const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// Encode turns a database ID into a short string.
func Encode(id int64) string {
	if id == 0 {
		return string(alphabet[0])
	}

	var s strings.Builder
	for id > 0 {
		s.WriteByte(alphabet[id%62])
		id /= 62
	}

	// The bytes are collected in reverse order, so we flip them.
	return reverse(s.String())
}

// Decode turns a short string back into a database ID.
func Decode(str string) (int64, error) {
	var res int64
	
	for _, char := range str {
		idx := strings.IndexRune(alphabet, char)
		if idx == -1 {
			return 0, errors.New("Invalid character!")
		}
		res = res*62 + int64(idx)
	}

	return res, nil
}

func reverse(str string) string {
	runes := []rune(str)

	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}