package models

type BaseModel struct {
	ID        uint  `gorm:"primary" json:"id"`
	CreatedAt int64 `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt int64 `gorm:"autoUpdateTime" json:"updated_at"`
}

// BackupConfig 备份配置
type BackupConfig struct {
	BaseModel
	BackupEnabled       int    `json:"backup_enabled" gorm:"default:0"`        // 是否启用自动备份，0表示禁用，1表示启用
	BackupCron          string `json:"backup_cron"`                            // 备份cron表达式
	BackupPath          string `json:"backup_path"`                            // 备份存储路径
	BackupRetention     int    `json:"backup_retention" gorm:"default:7"`      // 备份保留天数
	BackupMaxCount      int    `json:"backup_max_count" gorm:"default:10"`     // 最多保留的备份数量
	BackupCompress      int    `json:"backup_compress" gorm:"default:1"`       // 是否压缩备份，0表示不压缩，1表示压缩
	MaintenanceMode     int    `json:"maintenance_mode" gorm:"default:0"`      // 维护模式，0表示关闭，1表示开启
	MaintenanceModeTime int64  `json:"maintenance_mode_time" gorm:"default:0"` // 进入维护模式的时间戳
}

func (*BackupConfig) TableName() string {
	return "backup_config"
}

// BackupRecord 备份记录（历史记录）
type BackupRecord struct {
	BaseModel
	TaskID           uint    `json:"task_id"`           // 关联的任务ID
	Status           string  `json:"status"`            // 任务状态：completed/cancelled/timeout/failed
	FilePath         string  `json:"file_path"`         // 备份文件路径
	FileSize         int64   `json:"file_size"`         // 备份文件大小（字节）
	DatabaseSize     int64   `json:"database_size"`     // 数据库大小（字节）
	TableCount       int     `json:"table_count"`       // 表数量
	BackupDuration   int64   `json:"backup_duration"`   // 备份耗时（秒）
	BackupType       string  `json:"backup_type"`       // 备份类型：manual/auto
	CreatedReason    string  `json:"created_reason"`    // 创建原因
	FailureReason    string  `json:"failure_reason"`    // 失败原因
	CompressionRatio float64 `json:"compression_ratio"` // 压缩比例
	IsCompressed     int     `json:"is_compressed"`     // 是否已压缩，0表示否，1表示是
	CompletedAt      int64   `json:"completed_at"`      // 完成时间戳
}

func (*BackupRecord) TableName() string {
	return "backup_record"
}
