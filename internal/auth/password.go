// Package auth contains password hashing and JWT helpers used by the
// register / login / refresh / me handlers.
package auth

import "golang.org/x/crypto/bcrypt"

// PasswordCost is the bcrypt cost. 12 ≈ ~250ms per hash on a modern laptop —
// a reasonable defence against offline brute force, still cheap enough for
// the hot path on login.
const PasswordCost = 12

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), PasswordCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword reports whether the plaintext matches the stored hash.
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
