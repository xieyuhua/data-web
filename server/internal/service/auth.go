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

// AuthService 登录认证服务。
type AuthService struct {
	userRepo *repository.UserRepository
	roleRepo *repository.RoleRepository
	jwt      *auth.Manager
}

// LoginResult 登录成功后返回给前端的完整信息。
type LoginResult struct {
	Token       string     `json:"token"`
	ExpiresAt   time.Time  `json:"expires_at"`
	User        model.User `json:"user"`
	Role        model.Role `json:"role"`
	Permissions []string   `json:"permissions"`
	DataScope   string     `json:"data_scope"`
}

// NewAuthService 创建认证服务。
func NewAuthService(userRepo *repository.UserRepository, roleRepo *repository.RoleRepository, jwt *auth.Manager) *AuthService {
	return &AuthService{userRepo: userRepo, roleRepo: roleRepo, jwt: jwt}
}

// Login 账号密码登录：校验 bcrypt 密码哈希并签发 JWT。
func (s *AuthService) Login(username, password string) (*LoginResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, errors.New("请输入用户名和密码")
	}
	user, err := s.userRepo.FindByUsername(username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, errors.New("用户名或密码错误")
	}
	return s.loginUser(user)
}

// LoginByUserID 供企业微信授权登录使用：用户身份已由企业微信校验，这里只校验平台侧状态。
func (s *AuthService) LoginByUserID(userID uint64) (*LoginResult, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, errors.New("账号不存在")
	}
	return s.loginUser(user)
}

// loginUser 统一生成登录结果：校验状态、加载角色、签发 Token、更新登录时间。
func (s *AuthService) loginUser(user *model.User) (*LoginResult, error) {
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

// HashPassword 生成 bcrypt 密码哈希，密码长度至少 6 位。
func HashPassword(password string) (string, error) {
	if len(password) < 6 {
		return "", errors.New("密码至少需要 6 位")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashed), err
}
