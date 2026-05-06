package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestMakeJWT(t *testing.T) {
	userID := uuid.New()
	tokenSecret := "test-secret"
	expiresIn := 1 * time.Hour

	token, err := MakeJWT(userID, tokenSecret, expiresIn)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// ✅ CORRECTED: Decode and verify the issuer
	tokenObj, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(tokenSecret), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	claims, ok := tokenObj.Claims.(*jwt.RegisteredClaims)
	if !ok || !tokenObj.Valid {
		t.Fatal("invalid token claims")
	}

	// ✅ Verify the issuer in the decoded payload
	assert.Equal(t, "chirpy-access", claims.Issuer)
}

func TestValidateJWT(t *testing.T) {
	userID := uuid.New()
	tokenSecret := "test-secret"
	expiresIn := 1 * time.Hour

	// Create valid token
	token, err := MakeJWT(userID, tokenSecret, expiresIn)
	assert.NoError(t, err)

	// Validate valid token
	validatedID, err := ValidateJWT(token, tokenSecret)
	assert.NoError(t, err)
	assert.Equal(t, userID, validatedID)

	// Validate expired token
	expiredToken, err := MakeJWT(userID, tokenSecret, -1*time.Minute)
	assert.NoError(t, err)
	_, err = ValidateJWT(expiredToken, tokenSecret)
	assert.Error(t, err)

	// Validate with wrong secret
	_, err = ValidateJWT(token, "wrong-secret")
	assert.Error(t, err)
}
