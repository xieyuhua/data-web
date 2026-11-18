package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"biaoge-bi/internal/crypto"
	"biaoge-bi/internal/datasource"
	"biaoge-bi/internal/model"
	"biaoge-bi/internal/repository"
)

type DataSourceService struct {
	repo   *repository.Repo
	manager *datasource.Manager
	cipher *crypto.Cipher
}

func NewDataSourceService(repo *repository.Repo, manager *datasource.Manager, cipher *crypto.Cipher) *DataSourceService {
	return &DataSourceService{repo: repo, manager: manager, cipher: cipher}
}

type DataSourceInput struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	Password    string `json:"password"` // plaintext on write only
	DSNExtra    string `json:"dsnExtra"`
	OptionsJSON string `json:"optionsJson"`
}

func (in *DataSourceInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return errors.New("数据源名称不能为空")
	}
	switch in.Type {
	case model.DSMySQL, model.DSPostgres, model.DSSQLite, model.DSOracle:
	default:
		return errors.New("不支持的数据源类型")
	}
	if in.Type == model.DSSQLite {
		if strings.TrimSpace(in.Database) == "" {
			return errors.New("SQLite 需要填写数据库文件路径")
		}
	} else if strings.TrimSpace(in.Host) == "" || strings.TrimSpace(in.Database) == "" {
		return errors.New("请填写主机地址和数据库名")
	}
	if in.OptionsJSON != "" && !json.Valid([]byte(in.OptionsJSON)) {
		return errors.New("optionsJson 不是合法 JSON")
	}
	return nil
}

func (s *DataSourceService) Create(in DataSourceInput, userID uint) (*model.DataSource, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	enc, err := s.cipher.Encrypt(in.Password)
	if err != nil {
		return nil, err
	}
	ds := model.DataSource{
		Name:        in.Name,
		Type:        in.Type,
		Host:        in.Host,
		Port:        in.Port,
		Database:    in.Database,
		Username:    in.Username,
		PasswordEnc: enc,
		DSNExtra:    in.DSNExtra,
		OptionsJSON: in.OptionsJSON,
		CreatedByID: userID,
	}
	if err := s.repo.DB.Create(&ds).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errors.New("数据源名称已存在")
		}
		return nil, err
	}
	return &ds, nil
}

func (s *DataSourceService) Update(id uint, in DataSourceInput) (*model.DataSource, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	ds, err := s.repo.DataSourceByID(id)
	if err != nil {
		return nil, err
	}
	ds.Name = in.Name
	ds.Type = in.Type
	ds.Host = in.Host
	ds.Port = in.Port
	ds.Database = in.Database
	ds.Username = in.Username
	ds.DSNExtra = in.DSNExtra
	ds.OptionsJSON = in.OptionsJSON
	if in.Password != "" {
		enc, err := s.cipher.Encrypt(in.Password)
		if err != nil {
			return nil, err
		}
		ds.PasswordEnc = enc
	}
	if err := s.repo.DB.Save(ds).Error; err != nil {
		return nil, err
	}
	s.manager.Invalidate(id)
	return ds, nil
}

// Delete removes a datasource, refusing while datasets still depend on it.
func (s *DataSourceService) Delete(id uint) error {
	var refs int64
	if err := s.repo.DB.Model(&model.Dataset{}).Where("data_source_id = ?", id).Count(&refs).Error; err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("该数据源被 %d 个数据集引用，请先删除或迁移数据集", refs)
	}
	s.manager.Invalidate(id)
	return s.repo.DB.Delete(&model.DataSource{}, id).Error
}

func (s *DataSourceService) List() ([]model.DataSource, error) {
	return s.repo.ListDataSources()
}

func (s *DataSourceService) Get(id uint) (*model.DataSource, error) {
	return s.repo.DataSourceByID(id)
}

func (s *DataSourceService) TestUnsaved(ctx context.Context, in DataSourceInput) (string, error) {
	if err := in.validate(); err != nil {
		return "", err
	}
	ds := &model.DataSource{
		Type: in.Type, Host: in.Host, Port: in.Port,
		Database: in.Database, Username: in.Username, DSNExtra: in.DSNExtra,
	}
	return s.manager.Test(ds, in.Password)
}

func (s *DataSourceService) TestByID(ctx context.Context, id uint) error {
	return s.manager.TestByID(ctx, id)
}

// SetTestResult records a test outcome (used after unsaved test on existing id flow).
func (s *DataSourceService) MarkTested(id uint, ok bool, msg string) {
	now := time.Now()
	status := "success"
	if !ok {
		status = "failed"
	}
	_ = s.repo.DB.Model(&model.DataSource{}).Where("id = ?", id).Updates(map[string]any{
		"last_tested_at": &now, "last_test_status": status, "last_test_message": msg,
	})
}
