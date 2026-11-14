package authn

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type AccessClaims struct {
	jwt.RegisteredClaims
	SessionID string `json:"sid"`
}

func NewTokenManager(secret string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (m *TokenManager) AccessTTL() time.Duration  { return m.accessTTL }
func (m *TokenManager) RefreshTTL() time.Duration { return m.refreshTTL }

func (m *TokenManager) IssueAccess(userID, sessionID uuid.UUID) (string, error) {
	now := time.Now().UTC()
	claims := AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
			Issuer:    "lokasolve",
		},
		SessionID: sessionID.String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *TokenManager) ParseAccess(raw string) (uuid.UUID, uuid.UUID, error) {
	claims := &AccessClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("lokasolve"))
	if err != nil || !parsed.Valid {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid token")
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid token")
	}
	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid token")
	}
	return userID, sessionID, nil
}

func NewRefreshToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashRefresh(raw), nil
}

func HashRefresh(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Seal hides a successor refresh token for the short overlap window. The raw
// token is not stored. The seal key is the hash of the access-token secret.
func (m *TokenManager) Seal(raw string) (string, error) {
	gcm, err := m.sealer()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(raw), nil)), nil
}

func (m *TokenManager) Open(sealed string) (string, error) {
	gcm, err := m.sealer()
	if err != nil {
		return "", err
	}
	buf, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil || len(buf) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid seal")
	}
	plain, err := gcm.Open(nil, buf[:gcm.NonceSize()], buf[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("invalid seal")
	}
	return string(plain), nil
}

func (m *TokenManager) sealer() (cipher.AEAD, error) {
	key := sha256.Sum256(m.secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
