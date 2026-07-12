package activitypub

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/authsession"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccessAuthorizerAcceptsHS256JWT(t *testing.T) {
	secret := []byte("secret")
	req := httptest.NewRequest("GET", "/projects/project-1/outbox", nil)
	req.Header.Set("Authorization", "Bearer "+signedActivityPubToken(t, secret, jwt.SigningMethodHS256))
	authorizer := NewAccessAuthorizer(nil, secret, nil)

	actorID, ok, err := authorizer.actorIDFromJWT(context.Background(), req)

	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "user-1", actorID)
}

func TestAccessAuthorizerRejectsUnexpectedHMACJWTVariant(t *testing.T) {
	secret := []byte("secret")
	req := httptest.NewRequest("GET", "/projects/project-1/outbox", nil)
	req.Header.Set("Authorization", "Bearer "+signedActivityPubToken(t, secret, jwt.SigningMethodHS384))
	authorizer := NewAccessAuthorizer(nil, secret, nil)

	actorID, ok, err := authorizer.actorIDFromJWT(context.Background(), req)

	require.ErrorIs(t, err, ErrInvalidAuthorization)
	assert.True(t, ok)
	assert.Empty(t, actorID)
}

func signedActivityPubToken(t *testing.T, secret []byte, method jwt.SigningMethod) string {
	t.Helper()
	token := jwt.NewWithClaims(method, jwt.MapClaims{
		"sub": "user-1",
		"exp": 9999999999,
		"iss": authsession.Issuer,
		"aud": authsession.Audience,
	})
	raw, err := token.SignedString(secret)
	require.NoError(t, err)
	return raw
}
