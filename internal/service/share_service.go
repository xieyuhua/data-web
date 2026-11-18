package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
)

type ShareService struct {
	repo *repository.Repo
}

func NewShareService(repo *repository.Repo) *ShareService { return &ShareService{repo: repo} }

type ShareLinkView struct {
	ID        uint       `json:"id"`
	Token     string     `json:"token,omitempty"` // returned only at creation
	ExpiresAt *time.Time `json:"expiresAt"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"createdAt"`
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Create generates a share link; the raw token is returned once and never stored.
func (s *ShareService) Create(dashboardID uint, userID uint, expiresAt *time.Time) (*ShareLinkView, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	link := model.ShareLink{
		DashboardID: dashboardID,
		TokenHash:   hashToken(token),
		ExpiresAt:   expiresAt,
		Enabled:     true,
		CreatedByID: userID,
	}
	if err := s.repo.DB.Create(&link).Error; err != nil {
		return nil, "", err
	}
	return &ShareLinkView{
		ID: link.ID, Token: token, ExpiresAt: link.ExpiresAt,
		Enabled: link.Enabled, CreatedAt: link.CreatedAt,
	}, token, nil
}

func (s *ShareService) List(dashboardID uint) ([]ShareLinkView, error) {
	var links []model.ShareLink
	if err := s.repo.DB.Where("dashboard_id = ?", dashboardID).Order("id DESC").Find(&links).Error; err != nil {
		return nil, err
	}
	out := make([]ShareLinkView, 0, len(links))
	for _, l := range links {
		out = append(out, ShareLinkView{ID: l.ID, ExpiresAt: l.ExpiresAt, Enabled: l.Enabled, CreatedAt: l.CreatedAt})
	}
	return out, nil
}

func (s *ShareService) Revoke(id uint, dashboardID uint) error {
	return s.repo.DB.Where("id = ? AND dashboard_id = ?", id, dashboardID).Delete(&model.ShareLink{}).Error
}

// CreateForPush issues a share link with a short expiry for automated pushes.
// Returns the raw token; callers build the public URL.
func (s *ShareService) CreateForPush(dashboardID uint, userID uint, expireHours int) (string, error) {
	var expires *time.Time
	if expireHours > 0 {
		t := time.Now().Add(time.Duration(expireHours) * time.Hour)
		expires = &t
	}
	_, token, err := s.Create(dashboardID, userID, expires)
	if err != nil {
		return "", err
	}
	return token, nil
}

// Resolve validates a raw token and returns the dashboard.
func (s *ShareService) Resolve(token string) (*model.Dashboard, error) {
	if token == "" {
		return nil, errors.New("分享链接无效")
	}
	var link model.ShareLink
	if err := s.repo.DB.Where("token_hash = ?", hashToken(token)).First(&link).Error; err != nil {
		return nil, errors.New("分享链接不存在或已被撤销")
	}
	if !link.Enabled {
		return nil, errors.New("分享链接已被禁用")
	}
	if link.ExpiresAt != nil && time.Now().After(*link.ExpiresAt) {
		return nil, errors.New("分享链接已过期")
	}
	return s.repo.DashboardByID(link.DashboardID)
}

func (s *ShareService) ResolveDashboardID(token string) (uint, error) {
	d, err := s.Resolve(token)
	if err != nil {
		return 0, err
	}
	return d.ID, nil
}
