package database

import (
	"Q115-STRM/internal/helpers"
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DBManager 数据库管理器接口
type DBManager interface {
	Start(ctx context.Context) error
	Stop() error
	GetDB() *sql.DB
	HealthCheck() error
	Backup(ctx context.Context, backupPath string) error
	Restore(ctx context.Context, backupPath string) error
}

// Manager 统一数据库管理器
type Manager struct {
	impl   DBManager
	config *Config
	db     *sql.DB
}

type Config struct {
	Mode         string // "embedded" 或 "external"
	Host         string
	Port         int
	User         string
	Password     string
	DBName       string
	SSLMode      string
	LogDir       string
	DataDir      string
	BinaryPath   string
	MaxOpenConns int
	MaxIdleConns int
	External     bool
}

func NewManager(config *Config) *Manager {
	var impl DBManager

	if config.Mode == "embedded" {
		impl = NewEmbeddedManager(config)
	} else {
		impl = NewExternalManager(config)
	}

	return &Manager{
		impl:   impl,
		config: config,
	}
}

func (m *Manager) Start(ctx context.Context) error {
	helpers.AppLogger.Infof("启动数据库管理器 (模式: %s)", m.config.Mode)

	if err := m.impl.Start(ctx); err != nil {
		return err
	}

	m.db = m.impl.GetDB()

	// 配置连接池
	m.db.SetMaxOpenConns(m.config.MaxOpenConns)
	m.db.SetMaxIdleConns(m.config.MaxIdleConns)
	m.db.SetConnMaxLifetime(5 * time.Minute)

	helpers.AppLogger.Info("数据库管理器启动完成")
	return nil
}

func (m *Manager) Stop() error {
	helpers.AppLogger.Info("停止数据库管理器")

	if m.db != nil {
		m.db.Close()
	}

	return m.impl.Stop()
}

func (m *Manager) GetDB() *sql.DB {
	return m.db
}

func (m *Manager) HealthCheck() error {
	if m.db == nil {
		return fmt.Errorf("数据库未连接")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return m.db.PingContext(ctx)
}

func (m *Manager) Backup(ctx context.Context, backupPath string) error {
	return m.impl.Backup(ctx, backupPath)
}

func (m *Manager) Restore(ctx context.Context, backupPath string) error {
	return m.impl.Restore(ctx, backupPath)
}

// GetMode 获取数据库模式
func (m *Manager) GetMode() string {
	if m.config == nil {
		return ""
	}
	return m.config.Mode
}
