package pass

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
)

// tokenPayload is the QR payload. exp is a Unix timestamp in seconds.
type tokenPayload struct {
	PassID        string `json:"passId"`
	ClaimID       string `json:"claimId"`
	ItemID        string `json:"itemId"`
	CustodianID   string `json:"custodianId"`
	ReturnChannel string `json:"returnChannel"`
	Exp           int64  `json:"exp"`
}

// signToken serialises the payload to base64url JSON and appends the base64url
// HMAC-SHA256 signature: `<payload>.<signature>`.
func signToken(secret string, payload tokenPayload) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encoding pass token: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(encoded)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + signature, nil
}

// verifyToken checks the HMAC before decoding the payload. Any malformed or
// tampered token is errs.ErrInvalidInput. The constant-time comparison avoids
// leaking how much of a forged signature was correct.
func verifyToken(secret, token string) (tokenPayload, error) {
	var payload tokenPayload

	body, signature, ok := strings.Cut(token, ".")
	if !ok || body == "" || signature == "" {
		return payload, fmt.Errorf("malformed pass token: %w", errs.ErrInvalidInput)
	}
	provided, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return payload, fmt.Errorf("malformed pass signature: %w", errs.ErrInvalidInput)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return payload, fmt.Errorf("invalid pass signature: %w", errs.ErrInvalidInput)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return payload, fmt.Errorf("malformed pass payload: %w", errs.ErrInvalidInput)
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return payload, fmt.Errorf("malformed pass payload: %w", errs.ErrInvalidInput)
	}
	if payload.PassID == "" || payload.ClaimID == "" || payload.ItemID == "" ||
		payload.CustodianID == "" || payload.Exp == 0 {
		return payload, fmt.Errorf("incomplete pass payload: %w", errs.ErrInvalidInput)
	}
	return payload, nil
}

// hashToken is the only token representation that is persisted.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
