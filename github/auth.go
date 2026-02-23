package github

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Transport is an http.RoundTripper that authenticates using a GitHub App installation token.
type Transport struct {
	appID      int64
	privateKey *rsa.PrivateKey
	baseURL    string

	mu              sync.Mutex
	installationID  int64
	token           string
	tokenExpiration time.Time
}

// NewTransport creates a new Transport from an App ID and PEM-encoded private key.
func NewTransport(appID int64, privateKeyPEM []byte) (*Transport, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM private key")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		parsed, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("failed to parse private key (PKCS1: %v, PKCS8: %v)", err, err2)
		}
		var ok bool
		key, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not an RSA key")
		}
	}

	return &Transport{
		appID:      appID,
		privateKey: key,
		baseURL:    "https://api.github.com",
	}, nil
}

// RoundTrip adds an installation token to the request and sends it.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.getInstallationToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get installation token: %w", err)
	}

	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Accept", "application/vnd.github+json")

	return http.DefaultTransport.RoundTrip(req2)
}

// getInstallationToken returns a cached token, refreshing if expired.
func (t *Transport) getInstallationToken() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.token != "" && time.Now().Before(t.tokenExpiration.Add(-1*time.Minute)) {
		return t.token, nil
	}

	if t.installationID == 0 {
		id, err := t.fetchInstallationID()
		if err != nil {
			return "", err
		}
		t.installationID = id
	}

	token, expiration, err := t.fetchInstallationToken(t.installationID)
	if err != nil {
		return "", err
	}

	t.token = token
	t.tokenExpiration = expiration
	return t.token, nil
}

// generateJWT creates a JWT for GitHub App authentication (RS256).
func (t *Transport) generateJWT() (string, error) {
	now := time.Now()
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}
	payload := map[string]interface{}{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": t.appID,
	}

	headerJSON, _ := json.Marshal(header)
	payloadJSON, _ := json.Marshal(payload)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	signingInput := headerB64 + "." + payloadB64

	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(nil, t.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + sigB64, nil
}

// fetchInstallationID retrieves the first installation ID using the App JWT.
func (t *Transport) fetchInstallationID() (int64, error) {
	jwt, err := t.generateJWT()
	if err != nil {
		return 0, err
	}

	req, _ := http.NewRequest("GET", t.baseURL+"/app/installations", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return 0, fmt.Errorf("failed to get installations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("failed to get installations (status=%d): %s", resp.StatusCode, body)
	}

	var installations []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&installations); err != nil {
		return 0, fmt.Errorf("failed to parse installations response: %w", err)
	}
	if len(installations) == 0 {
		return 0, fmt.Errorf("no GitHub App installations found")
	}

	return installations[0].ID, nil
}

// fetchInstallationToken retrieves an installation access token.
func (t *Transport) fetchInstallationToken(installationID int64) (string, time.Time, error) {
	jwt, err := t.generateJWT()
	if err != nil {
		return "", time.Time{}, err
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", t.baseURL, installationID)
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get access token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("failed to get access token (status=%d): %s", resp.StatusCode, body)
	}

	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse access token response: %w", err)
	}

	return result.Token, result.ExpiresAt, nil
}
