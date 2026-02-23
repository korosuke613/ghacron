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

// Transport GitHub App認証付きHTTP RoundTripper
type Transport struct {
	appID      int64
	privateKey *rsa.PrivateKey
	baseURL    string

	mu              sync.Mutex
	installationID  int64
	token           string
	tokenExpiration time.Time
}

// NewTransport 新しいTransportを作成
func NewTransport(appID int64, privateKeyPEM []byte) (*Transport, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("PEM秘密鍵のデコードに失敗")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// PKCS8 形式も試行
		parsed, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("秘密鍵のパースに失敗 (PKCS1: %v, PKCS8: %v)", err, err2)
		}
		var ok bool
		key, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("秘密鍵がRSAキーではありません")
		}
	}

	return &Transport{
		appID:      appID,
		privateKey: key,
		baseURL:    "https://api.github.com",
	}, nil
}

// RoundTrip HTTPリクエストにInstallation Tokenを付与して送信
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.getInstallationToken()
	if err != nil {
		return nil, fmt.Errorf("Installation Token取得に失敗: %w", err)
	}

	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Accept", "application/vnd.github+json")

	return http.DefaultTransport.RoundTrip(req2)
}

// getInstallationToken キャッシュ済みトークンを返す。期限切れなら再取得
func (t *Transport) getInstallationToken() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// トークンが有効期限の1分前以降ならリフレッシュ
	if t.token != "" && time.Now().Before(t.tokenExpiration.Add(-1*time.Minute)) {
		return t.token, nil
	}

	// Installation IDが未取得なら取得
	if t.installationID == 0 {
		id, err := t.fetchInstallationID()
		if err != nil {
			return "", err
		}
		t.installationID = id
	}

	// Installation Token を取得
	token, expiration, err := t.fetchInstallationToken(t.installationID)
	if err != nil {
		return "", err
	}

	t.token = token
	t.tokenExpiration = expiration
	return t.token, nil
}

// generateJWT GitHub App用JWTを生成（RS256）
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
		return "", fmt.Errorf("JWT署名に失敗: %w", err)
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + sigB64, nil
}

// fetchInstallationID App JWTで最初のInstallation IDを取得
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
		return 0, fmt.Errorf("installations取得に失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("installations取得に失敗 (status=%d): %s", resp.StatusCode, body)
	}

	var installations []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&installations); err != nil {
		return 0, fmt.Errorf("installationsレスポンスのパースに失敗: %w", err)
	}
	if len(installations) == 0 {
		return 0, fmt.Errorf("GitHub Appのインストールが見つかりません")
	}

	return installations[0].ID, nil
}

// fetchInstallationToken Installation Access Tokenを取得
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
		return "", time.Time{}, fmt.Errorf("access_tokens取得に失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("access_tokens取得に失敗 (status=%d): %s", resp.StatusCode, body)
	}

	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("access_tokensレスポンスのパースに失敗: %w", err)
	}

	return result.Token, result.ExpiresAt, nil
}
