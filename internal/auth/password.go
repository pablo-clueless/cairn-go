package auth

import "golang.org/x/crypto/bcrypt"

// hashPassword returns a bcrypt hash of the plaintext password.
func hashPassword(plaintext string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// checkPassword reports whether plaintext matches the stored bcrypt hash.
func checkPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
