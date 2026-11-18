package service

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"biaoge-bi/internal/auth"
	"biaoge-bi/internal/config"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUserDisabled       = errors.New("账号已被禁用")
	ErrDuplicateUsername  = errors.New("用户名已存在")
	ErrWeakPassword       = errors.New("密码长度需在 8-128 位之间")
)

type AuthService struct {
	repo *repository.Repo
	jwt  *auth.JWTManager
	cfg  *config.Config
}

func NewAuthService(repo *repository.Repo, jwt *auth.JWTManager, cfg *config.Config) *AuthService {
	return &AuthService{repo: repo, jwt: jwt, cfg: cfg}
}

type LoginResult struct {
	Token string      `json:"token"`
	User  *model.User `json:"user"`
}

func (s *AuthService) Login(username, password string) (*LoginResult, error) {
	u, err := s.repo.UserByUsername(username)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if !auth.CheckPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	if !u.Enabled {
		return nil, ErrUserDisabled
	}
	token, err := s.jwt.Generate(u.ID, u.Username, u.Role)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	_ = s.repo.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("last_login_at", &now)
	u.LastLoginAt = &now
	return &LoginResult{Token: token, User: u}, nil
}

// issueToken mints a JWT for an already-authenticated user (used by SSO).
func (s *AuthService) issueToken(u *model.User) (*LoginResult, error) {
	token, err := s.jwt.Generate(u.ID, u.Username, u.Role)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	_ = s.repo.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("last_login_at", &now)
	u.LastLoginAt = &now
	return &LoginResult{Token: token, User: u}, nil
}

func (s *AuthService) ParseToken(token string) (*auth.Claims, *model.User, error) {
	claims, err := s.jwt.Parse(token)
	if err != nil {
		return nil, nil, errors.New("登录状态无效或已过期")
	}
	var userID uint
	for _, c := range claims.Subject {
		if c < '0' || c > '9' {
			return nil, nil, errors.New("token 载荷无效")
		}
		userID = userID*10 + uint(c-'0')
	}
	u, err := s.repo.UserByID(userID)
	if err != nil {
		return nil, nil, errors.New("用户不存在")
	}
	if !u.Enabled {
		return nil, nil, ErrUserDisabled
	}
	return claims, u, nil
}

func (s *AuthService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	u, err := s.repo.UserByID(userID)
	if err != nil {
		return err
	}
	if !auth.CheckPassword(u.PasswordHash, oldPassword) {
		return errors.New("原密码不正确")
	}
	if !auth.ValidPassword(newPassword) {
		return ErrWeakPassword
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.repo.DB.Model(&model.User{}).Where("id = ?", userID).Update("password_hash", hash).Error
}

// BootstrapAdmin creates the initial admin account when the user table is empty.
func (s *AuthService) BootstrapAdmin() error {
	n, err := s.repo.CountUsers()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	username := s.cfg.Auth.AdminUsername
	if username == "" {
		username = "admin"
	}
	password := s.cfg.Auth.AdminPassword
	generated := false
	if password == "" {
		var err error
		password, err = randomPassword()
		if err != nil {
			return err
		}
		generated = true
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	admin := model.User{
		Username:     username,
		PasswordHash: hash,
		DisplayName:  "Administrator",
		Role:         model.RoleAdmin,
		Enabled:      true,
	}
	if err := s.repo.DB.Create(&admin).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil
		}
		return err
	}
	if generated {
		slog.Warn("已创建初始管理员账号，请立即修改密码", "username", username, "password", password)
	} else {
		slog.Info("已创建初始管理员账号", "username", username)
	}
	return nil
}

// authHashRandom returns a bcrypt hash of a random secret, used for accounts
// that authenticate through SSO or imported placeholders.
func authHashRandom() (string, error) {
	p, err := randomPassword()
	if err != nil {
		return "", err
	}
	return auth.HashPassword(p)
}

func randomPassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ---------- Users ----------

type UserService struct {
	repo *repository.Repo
}

func NewUserService(repo *repository.Repo) *UserService { return &UserService{repo: repo} }

type CreateUserInput struct {
	Username    string `json:"username" binding:"required,min=2,max=64"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Role        string `json:"role"`
}

func (s *UserService) Create(in CreateUserInput) (*model.User, error) {
	if !auth.ValidPassword(in.Password) {
		return nil, ErrWeakPassword
	}
	if in.Role != model.RoleAdmin {
		in.Role = model.RoleUser
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u := model.User{
		Username:     strings.TrimSpace(in.Username),
		PasswordHash: hash,
		DisplayName:  in.DisplayName,
		Email:        in.Email,
		Role:         in.Role,
		Enabled:      true,
	}
	if err := s.repo.DB.Create(&u).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrDuplicateUsername
		}
		return nil, err
	}
	return &u, nil
}

type UpdateUserInput struct {
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	Role        *string `json:"role"`
	Enabled     *bool   `json:"enabled"`
}

func (s *UserService) Update(id uint, in UpdateUserInput) (*model.User, error) {
	u, err := s.repo.UserByID(id)
	if err != nil {
		return nil, err
	}
	if in.DisplayName != nil {
		u.DisplayName = *in.DisplayName
	}
	if in.Email != nil {
		u.Email = *in.Email
	}
	if in.Role != nil {
		if *in.Role == model.RoleAdmin || *in.Role == model.RoleUser {
			u.Role = *in.Role
		}
	}
	if in.Enabled != nil {
		u.Enabled = *in.Enabled
	}
	if err := s.repo.DB.Save(u).Error; err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserService) ResetPassword(id uint, newPassword string) error {
	if !auth.ValidPassword(newPassword) {
		return ErrWeakPassword
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.repo.DB.Model(&model.User{}).Where("id = ?", id).Update("password_hash", hash).Error
}

func (s *UserService) Delete(id uint) error {
	return s.repo.DB.Delete(&model.User{}, id).Error
}

func (s *UserService) List(keyword string, page, pageSize int) ([]model.User, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return s.repo.ListUsers(keyword, page, pageSize)
}

func (s *UserService) Get(id uint) (*model.User, error) {
	return s.repo.UserByID(id)
}

func ensurePositiveID(id uint) error {
	if id == 0 {
		return fmt.Errorf("无效的 ID")
	}
	return nil
}
