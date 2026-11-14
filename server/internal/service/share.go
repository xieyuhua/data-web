package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"bi-platform/internal/config"
	"bi-platform/internal/model"
	"bi-platform/internal/repository"
)

type ShareService struct {
	repo     *repository.ShareRepository
	dashRepo *repository.DashboardRepository
	cfg      config.Config
}

type ShareCreated struct {
	ID        uint64    `json:"id"`
	Token     string    `json:"token"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxViews  int       `json:"max_views"`
}

func NewShareService(repo *repository.ShareRepository, dashRepo *repository.DashboardRepository, cfg config.Config) *ShareService {
	return &ShareService{repo: repo, dashRepo: dashRepo, cfg: cfg}
}

// Create 生成分享链接。baseURL 为空时按配置自动推断（WebBaseURL → APIBaseURL → localhost:Port）。
func (s *ShareService) Create(userID, dashboardID uint64, orgIDs []uint64, expireHours, maxViews int, baseURL string) (*ShareCreated, error) {
	if _, err := s.dashRepo.FindByID(dashboardID); err != nil {
		return nil, errors.New("看板不存在")
	}
	if expireHours <= 0 || expireHours > 24*365 {
		expireHours = 24
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	item := &model.ShareLink{
		TokenHash: hashToken(token), DashboardID: dashboardID, OrgIDs: joinIDs(orgIDs),
		CreatedBy: userID, ExpiresAt: now.Add(time.Duration(expireHours) * time.Hour), MaxViews: maxViews,
	}
	if err := s.repo.Create(item); err != nil {
		return nil, err
	}
	return &ShareCreated{
		ID: item.ID, Token: token, URL: s.absoluteURL(baseURL, token),
		ExpiresAt: item.ExpiresAt, MaxViews: item.MaxViews,
	}, nil
}

// absoluteURL 优先使用请求上下文推断出的地址，其次使用配置，最终回退到本机端口。
func (s *ShareService) absoluteURL(baseURL, token string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(s.cfg.WebBaseURL), "/")
	}
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(s.cfg.APIBaseURL), "/")
	}
	if base == "" {
		base = "http://localhost:" + s.cfg.Port
	}
	return base + "/share/" + token
}

func (s *ShareService) Resolve(token string) (*model.ShareLink, []uint64, error) {
	return s.resolve(token, true)
}

// ResolveForRead 校验分享链接但不计访问次数，用于同一会话内的下钻等二次请求。
func (s *ShareService) ResolveForRead(token string) (*model.ShareLink, []uint64, error) {
	return s.resolve(token, false)
}

func (s *ShareService) resolve(token string, countView bool) (*model.ShareLink, []uint64, error) {
	item, err := s.repo.FindByHash(hashToken(token))
	if err != nil {
		return nil, nil, errors.New("分享链接无效")
	}
	if item.Revoked {
		return nil, nil, errors.New("分享链接已撤销")
	}
	if time.Now().After(item.ExpiresAt) {
		return nil, nil, errors.New("分享链接已过期")
	}
	if item.MaxViews > 0 && item.Views >= item.MaxViews {
		return nil, nil, errors.New("分享链接访问次数已达上限")
	}
	if countView {
		if err := s.repo.IncrementViews(item.ID); err != nil {
			return nil, nil, err
		}
	}
	return item, parseIDs(item.OrgIDs), nil
}

func (s *ShareService) List(userID uint64, isAdmin bool) ([]model.ShareLink, error) {
	return s.repo.ListByUser(userID, isAdmin)
}

func (s *ShareService) Revoke(id, userID uint64, isAdmin bool) error {
	return s.repo.Revoke(id, userID, isAdmin)
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func joinIDs(ids []uint64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(id, 10))
	}
	return strings.Join(parts, ",")
}

func parseIDs(value string) []uint64 {
	parts := strings.Split(value, ",")
	result := make([]uint64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			result = append(result, id)
		}
	}
	return result
}

var _ = fmt.Sprintf
