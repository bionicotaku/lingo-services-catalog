// Package e2e provides end-to-end integration test utilities.
package e2e

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTTestHelper 提供测试用的 JWT Token 生成和验证工具。
type JWTTestHelper struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	issuer     string
}

// NewJWTTestHelper 创建测试用的 JWT 工具，生成 RSA 密钥对。
func NewJWTTestHelper(t *testing.T, issuer string) *JWTTestHelper {
	t.Helper()

	// 生成 2048 位 RSA 密钥对
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	return &JWTTestHelper{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		issuer:     issuer,
	}
}

// CloudRunClaims 模拟 Cloud Run OIDC Token 的 Claims 结构。
// 必须匹配 gcjwt.CloudRunClaims 的 JSON 标签。
type CloudRunClaims struct {
	Sub   string `json:"sub"`
	Aud   string `json:"aud"`
	Email string `json:"email"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
	Iss   string `json:"iss,omitempty"`
	Azp   string `json:"azp,omitempty"`
}

// Valid 实现 jwt.Claims 接口。
func (c CloudRunClaims) Valid() error {
	now := time.Now().Unix()
	if c.Exp > 0 && now >= c.Exp {
		return fmt.Errorf("token expired")
	}
	return nil
}

// GetExpirationTime 实现 jwt.Claims 接口。
func (c CloudRunClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.Exp == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.Exp, 0)), nil
}

// GetIssuedAt 实现 jwt.Claims 接口。
func (c CloudRunClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	if c.Iat == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.Iat, 0)), nil
}

// GetNotBefore 实现 jwt.Claims 接口。
func (c CloudRunClaims) GetNotBefore() (*jwt.NumericDate, error) {
	return nil, nil
}

// GetIssuer 实现 jwt.Claims 接口。
func (c CloudRunClaims) GetIssuer() (string, error) {
	return c.Iss, nil
}

// GetSubject 实现 jwt.Claims 接口。
func (c CloudRunClaims) GetSubject() (string, error) {
	return c.Sub, nil
}

// GetAudience 实现 jwt.Claims 接口。
func (c CloudRunClaims) GetAudience() (jwt.ClaimStrings, error) {
	return jwt.ClaimStrings{c.Aud}, nil
}

// GenerateToken 生成自签名的 JWT Token。
//
// 参数：
//   - audience: Token 的目标受众（例如 https://my-service.run.app/）
//   - email: Service Account email（例如 service-a@project.iam.gserviceaccount.com）
//   - expiry: Token 过期时间（建议 1 小时）
func (h *JWTTestHelper) GenerateToken(audience, email string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := CloudRunClaims{
		Sub:   email,
		Aud:   audience,
		Email: email,
		Iat:   now.Unix(),
		Exp:   now.Add(expiry).Unix(),
		Iss:   h.issuer,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(h.privateKey)
}

// GetPublicKeyPEM 返回 PEM 格式的公钥（用于验证）。
func (h *JWTTestHelper) GetPublicKeyPEM() (string, error) {
	pubKeyBytes, err := json.Marshal(h.publicKey)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(pubKeyBytes), nil
}

// VerifyToken 验证 Token 并返回 Claims（用于测试验证）。
func (h *JWTTestHelper) VerifyToken(tokenString, expectedAudience string) (*CloudRunClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CloudRunClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名算法
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return h.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*CloudRunClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	// 验证 audience
	if expectedAudience != "" && claims.Aud != expectedAudience {
		return nil, fmt.Errorf("audience mismatch: expected %s, got %s", expectedAudience, claims.Aud)
	}

	return claims, nil
}

// GenerateValidCloudRunToken 生成符合 Cloud Run 格式的测试 Token。
//
// 这个函数创建一个完整的 JWT Token，包含：
// - 正确的 header (alg: RS256, typ: JWT)
// - 完整的 Claims (sub, aud, email, iat, exp)
// - 使用 RSA 私钥签名
//
// 使用场景：
// - 测试 skip_validate=true 模式（只需要正确的 JWT 格式，不需要 Google 签名）
// - 本地开发环境
func GenerateValidCloudRunToken(t *testing.T, audience, email string) string {
	t.Helper()

	helper := NewJWTTestHelper(t, "https://accounts.google.com")
	token, err := helper.GenerateToken(audience, email, 1*time.Hour)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	return token
}
