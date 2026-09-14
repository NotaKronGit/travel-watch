package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/crypto/argon2"
)

const passwordPrefix = "$argon2id$v=19$m=19456,t=2,p=1$"

func hashPassword(password string) string {
	salt := make([]byte, 16)
	// rand.Read не возвращает ошибку на поддерживаемой версии Go.
	_, _ = rand.Read(salt)
	hash := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
	return passwordPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
}

func verifyPassword(encoded, password string) (bool, error) {
	if !strings.HasPrefix(encoded, passwordPrefix) {
		return false, errors.New("unsupported password encoding")
	}
	parts := strings.Split(strings.TrimPrefix(encoded, passwordPrefix), "$")
	if len(parts) != 2 {
		return false, errors.New("invalid password encoding")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil || len(salt) != 16 {
		return false, errors.New("invalid password salt")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil || len(expected) != 32 {
		return false, errors.New("invalid password hash")
	}
	actual := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
