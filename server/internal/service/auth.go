package service

import (
	"errors"
	"strings"
	"time"

	"bi-platform/internal/model"
	"bi-platform/internal/pkg/auth"
	"bi-platform/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo *repository.UserRepository
	roleRepo *repository.RoleRepository
	jwt      *auth.Manager
}

type LoginResult struct {
	Token       string     `json:"token"`
	ExpiresAt   time.Time  `json:"expires_at"`
	User        model.User `json:"user"`
	Role        model.Role `json:"role"`
	Permissions []string   `json:"permissions"`
	DataScope   string     `json:"data_scope"`
}

func NewAuthService(userRepo *repository.UserRepository, roleRepo *repository.RoleRepository, jwt *auth.Manager) *AuthService {
	return &AuthService{userRepo: userRepo, roleRepo: roleRepo, jwt: jwt}
}

func (s *AuthService) Login(username, password string) (*LoginResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, errors.New("请输入用户名和密码")
	}
	user, err := s.userRepo.FindByUsername(username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, errors.New("用户名或密码错误")
	}
	if user.Status != "active" {
		return nil, errors.New("账号已被停用")
	}
	role, err := s.roleRepo.FindByCode(user.RoleCode)
	if err != nil {
		return nil, errors.New("账号角色配置异常")
	}
	token, err := s.jwt.Generate(user.ID, user.Name, user.RoleCode)
	if err != nil {
		return nil, err
	}
	_ = s.userRepo.UpdateLastLogin(user.ID)
	org, _ := s.userRepo.FindByID(user.ID)
	if org != nil {
		user.Organization = org.Organization
	}
	return &LoginResult{
		Token:       token,
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		User:        *user,
		Role:        *role,
		Permissions: Permissions(role),
		DataScope:   role.DataScope,
	}, nil
}

func HashPassword(password string) (string, error) {
	if len(password) < 6 {
		return "", errors.New("密码至少需要 6 位")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashed), err
}
