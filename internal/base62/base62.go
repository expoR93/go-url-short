package base62

import (
	"errors"
	"strings"
)

// The character set for Base62
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Encode turns a database ID into a short string.
func Encode(id uint64) string {
	if id == 0 {
		return string(alphabet[0])
	}

	// Max uint64 in base62 is 11 chars. Pre-allocating avoids extra heap objects.
	res := make([]byte, 0, 11)
	for id > 0 {
		res = append(res, alphabet[id%62])
		id /= 62
	}

	// Reverse in-place to avoid the extra overhead of a helper function and allocations
	for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
		res[i], res[j] = res[j], res[i]
	}

	return string(res)
}

// Decode turns a short string back into a database ID.
func Decode(str string) (uint64, error) {
	var res uint64

	for _, char := range str {
		idx := strings.IndexRune(alphabet, char)
		if idx == -1 {
			return 0, errors.New("Invalid character")
		}
		res = res*62 + uint64(idx)
	}

	return res, nil
}
