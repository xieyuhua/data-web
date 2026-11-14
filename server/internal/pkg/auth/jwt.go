package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID uint64 `json:"uid"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// Manager 负责签发与校验登录 JWT、短时效凭据。
type Manager struct {
	secret []byte
	expire time.Duration
}

// EphemeralClaims 用于短时效的一次性凭据：企业微信 OAuth state、登录 ticket、绑定 ticket。
// 与登录 JWT 使用同一签名密钥，但通过 Kind 严格区分用途，避免被当作登录凭证使用。
type EphemeralClaims struct {
	Kind     string `json:"kind"`
	Redirect string `json:"redirect,omitempty"`
	Mode     string `json:"mode,omitempty"`
	UserID   uint64 `json:"uid,omitempty"`
	jwt.RegisteredClaims
}

// NewManager 创建 JWT 管理器。
func NewManager(secret string, expire time.Duration) *Manager {
	return &Manager{secret: []byte(secret), expire: expire}
}

// Generate 签发正式登录 JWT（默认 24 小时，可由配置调整）。
func (m *Manager) Generate(userID uint64, name, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Name:   name,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "bi-platform",
			Subject:   name,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expire)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse 校验登录 JWT 并返回声明；只接受 HS256，防止算法混淆攻击。
func (m *Manager) Parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("无效的签名算法")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("登录凭证无效")
	}
	return claims, nil
}

// SignEphemeral 签发短时效凭据（state / ticket），默认 10 分钟。
func (m *Manager) SignEphemeral(claims EphemeralClaims, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	now := time.Now()
	claims.Issuer = "bi-platform"
	claims.IssuedAt = jwt.NewNumericDate(now)
	claims.ExpiresAt = jwt.NewNumericDate(now.Add(ttl))
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// ParseEphemeral 校验短时效凭据签名与有效期。
func (m *Manager) ParseEphemeral(token string) (*EphemeralClaims, error) {
	parsed, err := jwt.ParseWithClaims(token, &EphemeralClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("无效的签名算法")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*EphemeralClaims)
	if !ok || !parsed.Valid || claims.Kind == "" {
		return nil, errors.New("凭据无效或已过期")
	}
	return claims, nil
}
