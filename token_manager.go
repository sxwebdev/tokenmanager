package tokenmanager

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Data stores the token-related data.
type Data[TAdditionalData any] struct {
	UserID         string          `json:"user_id"`
	IssuedAt       time.Time       `json:"issued_at"`
	Expiry         time.Time       `json:"expiry"`
	TokenType      TokenType       `json:"token_type"`
	AdditionalData TAdditionalData `json:"additional_data"`
}

// Manager uses an ITokenStore for managing token state.
// It can create, validate, and revoke tokens.
type Manager[TAdditionalData any] struct {
	store         ITokenStore
	secretKey     []byte
	tokenDuration time.Duration
}

// New returns a new token manager with the provided storage and secret key.
func New[TAdditionalData any](store ITokenStore, secretKey string, tokenDuration time.Duration) *Manager[TAdditionalData] {
	return &Manager[TAdditionalData]{
		store:         store,
		secretKey:     []byte(secretKey),
		tokenDuration: tokenDuration,
	}
}

// createSignedToken generates a random payload and computes its HMAC-SHA256 signature.
func (tm *Manager[TAdditionalData]) createSignedToken() (payload string, signedToken string, err error) { //nolint:nonamedreturns
	randomBytes := make([]byte, 64)
	if _, err = rand.Read(randomBytes); err != nil {
		return "", "", err
	}
	payload = hex.EncodeToString(randomBytes)
	mac := hmac.New(sha256.New, tm.secretKey)
	mac.Write(randomBytes)
	signature := mac.Sum(nil)
	signatureHex := hex.EncodeToString(signature)
	signedToken = fmt.Sprintf("%s.%s", payload, signatureHex)
	return payload, signedToken, nil
}

// CreateToken generates a new token of the specified type, saves the token data into the storage,
// and returns the signed token.
func (tm *Manager[TAdditionalData]) CreateToken(
	ctx context.Context,
	userID string,
	additionalData TAdditionalData,
	tokenType TokenType,
) (string, Data[TAdditionalData], error) {
	payload, signedToken, err := tm.createSignedToken()
	if err != nil {
		return "", Data[TAdditionalData]{}, err
	}
	now := time.Now().UTC()
	td := Data[TAdditionalData]{
		UserID:         userID,
		IssuedAt:       now,
		Expiry:         now.Add(tm.tokenDuration),
		TokenType:      tokenType,
		AdditionalData: additionalData,
	}
	data, err := json.Marshal(td)
	if err != nil {
		return "", Data[TAdditionalData]{}, err
	}
	// Use the payload as the key.
	if err := tm.store.Set(ctx, getKey(payload), data, tm.tokenDuration); err != nil {
		return "", Data[TAdditionalData]{}, err
	}
	return signedToken, td, nil
}

// verifyAndExtractPayload validates the token format and HMAC signature,
// returning the payload hex string on success.
func (tm *Manager[TAdditionalData]) verifyAndExtractPayload(signedToken string) (string, error) {
	if signedToken == "" {
		return "", errors.New("token cannot be empty")
	}
	parts := strings.Split(signedToken, ".")
	if len(parts) != 2 {
		return "", errors.New("invalid token format")
	}
	payloadHex := parts[0]
	providedSigHex := parts[1]

	payloadBytes, err := hex.DecodeString(payloadHex)
	if err != nil {
		return "", fmt.Errorf("invalid token payload: %w", err)
	}
	mac := hmac.New(sha256.New, tm.secretKey)
	mac.Write(payloadBytes)
	expectedSig := mac.Sum(nil)
	expectedSigHex := hex.EncodeToString(expectedSig)
	if !hmac.Equal([]byte(providedSigHex), []byte(expectedSigHex)) {
		return "", errors.New("invalid token signature")
	}
	return payloadHex, nil
}

// ValidateToken checks that the token has a valid format, that its signature is correct,
// that the corresponding data can be retrieved from storage, and that it matches the expected type and is not expired.
func (tm *Manager[TAdditionalData]) ValidateToken(ctx context.Context, signedToken string, expectedType TokenType) (*Data[TAdditionalData], bool) {
	payloadHex, err := tm.verifyAndExtractPayload(signedToken)
	if err != nil {
		return nil, false
	}
	data, err := tm.store.Get(ctx, getKey(payloadHex))
	if err != nil {
		return nil, false
	}
	var td Data[TAdditionalData]
	if err = json.Unmarshal(data, &td); err != nil {
		return nil, false
	}
	if td.TokenType != expectedType {
		return nil, false
	}
	if time.Now().UTC().After(td.Expiry) {
		return nil, false
	}
	return &td, true
}

// RevokeToken verifies the token signature and removes it from the storage.
func (tm *Manager[TAdditionalData]) RevokeToken(ctx context.Context, signedToken string) error {
	payloadHex, err := tm.verifyAndExtractPayload(signedToken)
	if err != nil {
		return err
	}
	return tm.store.Delete(ctx, getKey(payloadHex))
}

// UpdateAdditionalData verifies the token signature and updates the token's additional data in the storage.
// The original TTL is preserved (not reset).
func (tm *Manager[TAdditionalData]) UpdateAdditionalData(
	ctx context.Context,
	signedToken string,
	newAdditionalData TAdditionalData,
) error {
	payloadHex, err := tm.verifyAndExtractPayload(signedToken)
	if err != nil {
		return err
	}

	// Retrieve the token data from storage.
	data, err := tm.store.Get(ctx, getKey(payloadHex))
	if err != nil {
		return err
	}
	var td Data[TAdditionalData]
	if err = json.Unmarshal(data, &td); err != nil {
		return err
	}

	remaining := time.Until(td.Expiry)
	if remaining <= 0 {
		return errors.New("token has expired")
	}

	// Update the additional data.
	td.AdditionalData = newAdditionalData
	data, err = json.Marshal(td)
	if err != nil {
		return err
	}

	// Save the updated token data back to storage with the remaining TTL.
	return tm.store.Set(ctx, getKey(payloadHex), data, remaining)
}
