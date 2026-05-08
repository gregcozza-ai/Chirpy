package auth

import (
	"github.com/alexedwards/argon2id"
	
)

// HashPassword hashes the password using Argon2id.
func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

// CheckPasswordHash compares the password with the stored hash.
func CheckPasswordHash(password, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, hash)
}


