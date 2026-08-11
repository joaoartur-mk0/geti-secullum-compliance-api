package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	const op = "auth.HashPassword"
	if password == "" {
		return "", fmt.Errorf("%s | Missing parameter | password is empty", op)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("%s | %w", op, err)
	}

	return string(hashed), nil
}
func CheckPassword(hash, password string) error {
	const op = "auth.CheckPassword"

	if hash == "" {
		return fmt.Errorf("%s | Missing parameter | hashedPassword is empty", op)
	}

	if password == "" {
		return fmt.Errorf("%s | Missing parameter | password is empty", op)
	}

	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return fmt.Errorf("%s | %w", op, err)
	}

	return nil
}
