package backup

import (
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"compress/gzip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RestoreHooks defines optional callbacks used during restore to pause/resume other services.
type RestoreHooks struct {
	StopCron            func()
	StartCron           func()
	PauseSyncQueue      func()
	ResumeSyncQueue     func()
	StopUploadQueue     func()
	ResumeUploadQueue   func()
	StopDownloadQueue   func()
	ResumeDownloadQueue func()
}

// ValidateCronInterval checks if cron expression runs no more frequently than hourly.
func ValidateCronInterval(cronExpr string) bool {
	times := helpers.GetNextTimeByCronStr(cronExpr, 2)
	if len(times) < 2 {
		return false
	}
	interval := times[1].Sub(times[0])
	return interval >= time.Hour
}

// ValidateBackupFile performs a lightweight check on backup file validity.
func ValidateBackupFile(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}
	ext := filepath.Ext(filePath)
	if ext == ".gz" {
		file, err := os.Open(filePath)
		if err != nil {
			return false
		}
		defer file.Close()
		header := make([]byte, 2)
		if _, err := file.Read(header); err != nil {
			return false
		}
		return header[0] == 0x1f && header[1] == 0x8b
	}
	return true
}

// PerformManualBackup runs a manual backup using in-memory runtime task tracking.
func PerformManualBackup(config *models.BackupConfig) {
	defer func() {
		if r := recover(); r != nil {
			helpers.AppLogger.Errorf("备份任务发生异常: %v", r)
			models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
				t.Status = "failed"
				t.FailureReason = fmt.Sprintf("%v", r)
				t.EndTime = time.Now().Unix()
			})
		}
	}()

	backupDir := filepath.Join(helpers.ConfigDir, config.BackupPath)
	_ = os.MkdirAll(backupDir, 0755)

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	sqlFileName := fmt.Sprintf("backup_%s.sql", timestamp)
	sqlFilePath := filepath.Join(backupDir, sqlFileName)

	models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
		t.FilePath = sqlFileName
		t.CurrentStep = "正在导出数据库..."
	})

	tableCount, dbSize, err := ExportDatabaseToSQL(sqlFilePath)
	if err != nil {
		helpers.AppLogger.Errorf("数据库备份失败: %v", err)
		models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
			t.Status = "failed"
			t.FailureReason = err.Error()
			t.EndTime = time.Now().Unix()
		})
		return
	}

	fileInfo, _ := os.Stat(sqlFilePath)
	models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
		t.FileSize = fileInfo.Size()
		t.Progress = 70
	})

	if config.BackupCompress == 1 {
		models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
			t.CurrentStep = "正在压缩备份文件..."
		})
		gzFilePath := sqlFilePath + ".gz"
		if err := compressFile(sqlFilePath, gzFilePath); err != nil {
			helpers.AppLogger.Errorf("压缩备份失败: %v", err)
			models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
				t.Status = "failed"
				t.FailureReason = "压缩失败: " + err.Error()
				t.EndTime = time.Now().Unix()
			})
			_ = os.Remove(sqlFilePath)
			return
		}
		_ = os.Remove(sqlFilePath)
		gzInfo, _ := os.Stat(gzFilePath)
		compressedSize := gzInfo.Size()
		models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
			t.FilePath = sqlFileName + ".gz"
			if t.FileSize > 0 {
				t.CompressionRatio = float64(compressedSize) / float64(t.FileSize)
			}
			t.FileSize = compressedSize
		})
	}

	models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
		t.Progress = 90
		t.CurrentStep = "正在保存备份记录..."
	})

	finalTask := models.GetCurrentBackupTask()
	duration := time.Now().Unix() - finalTask.StartTime
	record := &models.BackupRecord{
		Status:           "completed",
		FilePath:         finalTask.FilePath,
		FileSize:         finalTask.FileSize,
		TableCount:       tableCount,
		DatabaseSize:     dbSize,
		BackupType:       "manual",
		CreatedReason:    finalTask.CreatedReason,
		BackupDuration:   duration,
		CompressionRatio: finalTask.CompressionRatio,
		IsCompressed:     config.BackupCompress,
		CompletedAt:      time.Now().Unix(),
	}
	if err := db.Db.Create(record).Error; err != nil {
		helpers.AppLogger.Errorf("保存备份记录失败: %v", err)
	}

	cleanupOldBackups(config)

	models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
		t.Status = "completed"
		t.Progress = 100
		t.CurrentStep = "备份完成"
		t.EndTime = time.Now().Unix()
	})

	helpers.AppLogger.Infof("备份任务完成，文件: %s", finalTask.FilePath)
}

// PerformRestore restores database from backup file using provided hooks.
func PerformRestore(backupFilePath string, hooks *RestoreHooks) {
	defer func() {
		if r := recover(); r != nil {
			helpers.AppLogger.Errorf("恢复任务发生异常: %v", r)
			models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
				t.Status = "failed"
				t.FailureReason = fmt.Sprintf("%v", r)
				t.EndTime = time.Now().Unix()
			})
		}
		if hooks != nil && hooks.ResumeUploadQueue != nil {
			hooks.ResumeUploadQueue()
		}
		if hooks != nil && hooks.ResumeDownloadQueue != nil {
			hooks.ResumeDownloadQueue()
		}
		if hooks != nil && hooks.StartCron != nil {
			hooks.StartCron()
		}
		if hooks != nil && hooks.ResumeSyncQueue != nil {
			hooks.ResumeSyncQueue()
		}
	}()

	if hooks != nil && hooks.StopCron != nil {
		hooks.StopCron()
	}
	if hooks != nil && hooks.StopUploadQueue != nil {
		hooks.StopUploadQueue()
	}
	if hooks != nil && hooks.StopDownloadQueue != nil {
		hooks.StopDownloadQueue()
	}
	if hooks != nil && hooks.PauseSyncQueue != nil {
		hooks.PauseSyncQueue()
	}

	sqlDB, err := db.Db.DB()
	if err != nil {
		helpers.AppLogger.Errorf("获取数据库连接失败: %v", err)
		models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
			t.Status = "failed"
			t.FailureReason = fmt.Sprintf("获取数据库连接失败: %v", err)
			t.EndTime = time.Now().Unix()
		})
		_ = os.Remove(backupFilePath)
		return
	}

	factory := NewDriverFactory(getDatabaseType(), sqlDB)
	driver := factory.CreateDriver()

	models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
		t.CurrentStep = "正在清空表数据..."
		t.Progress = 10
	})
	if err := driver.TruncateAllTables(); err != nil {
		helpers.AppLogger.Errorf("清空表失败: %v", err)
		models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
			t.Status = "failed"
			t.FailureReason = fmt.Sprintf("清空表失败: %v", err)
			t.EndTime = time.Now().Unix()
		})
		_ = os.Remove(backupFilePath)
		return
	}

	models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
		t.CurrentStep = "正在导入数据..."
		t.Progress = 50
	})

	file, err := os.Open(backupFilePath)
	if err != nil {
		helpers.AppLogger.Errorf("打开备份文件失败: %v", err)
		models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
			t.Status = "failed"
			t.FailureReason = fmt.Sprintf("打开备份文件失败: %v", err)
			t.EndTime = time.Now().Unix()
		})
		_ = os.Remove(backupFilePath)
		return
	}
	defer file.Close()

	var reader io.Reader = file
	if filepath.Ext(backupFilePath) == ".gz" {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			helpers.AppLogger.Errorf("打开压缩文件失败: %v", err)
			models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
				t.Status = "failed"
				t.FailureReason = fmt.Sprintf("打开压缩文件失败: %v", err)
				t.EndTime = time.Now().Unix()
			})
			_ = os.Remove(backupFilePath)
			return
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	if err := driver.ImportFromSQL(reader); err != nil {
		helpers.AppLogger.Errorf("导入数据失败: %v", err)
		models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
			t.Status = "failed"
			t.FailureReason = fmt.Sprintf("导入数据失败: %v", err)
			t.EndTime = time.Now().Unix()
		})
		_ = os.Remove(backupFilePath)
		return
	}

	_ = os.Remove(backupFilePath)

	models.UpdateRestoreTask(func(t *models.RuntimeRestoreTask) {
		t.Status = "completed"
		t.Progress = 100
		t.EndTime = time.Now().Unix()
	})
	helpers.AppLogger.Info("数据库恢复完成")
}

// ExportDatabaseToSQL dumps database to a SQL file.
func ExportDatabaseToSQL(filePath string) (int, int64, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	sqlDB, err := db.Db.DB()
	if err != nil {
		return 0, 0, fmt.Errorf("获取数据库连接失败: %v", err)
	}

	var tables []string
	if err := db.Db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'").Scan(&tables).Error; err != nil {
		return 0, 0, fmt.Errorf("获取表列表失败: %v", err)
	}
	if len(tables) == 0 {
		return 0, 0, fmt.Errorf("数据库中没有表")
	}

	tableCount := len(tables)
	var dbSize int64
	if err := db.Db.Raw("SELECT pg_database_size(current_database())").Scan(&dbSize).Error; err != nil {
		helpers.AppLogger.Warnf("获取数据库大小失败: %v，使用备份文件大小", err)
		dbSize = 0
	}

	file.WriteString("-- Database backup\n")
	file.WriteString(fmt.Sprintf("-- Generated at %s\n", time.Now().Format("2006-01-02 15:04:05")))
	file.WriteString(fmt.Sprintf("-- Tables: %d\n\n", tableCount))

	for _, tableName := range tables {
		rows, err := sqlDB.Query(fmt.Sprintf("SELECT * FROM \"%s\"", tableName))
		if err != nil {
			helpers.AppLogger.Warnf("读取表%s失败: %v", tableName, err)
			continue
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			helpers.AppLogger.Warnf("获取表%s列信息失败: %v", tableName, err)
			continue
		}
		if len(columns) > 0 {
			file.WriteString(fmt.Sprintf("-- Table: %s\n", tableName))
			columnNames := make([]string, len(columns))
			for i, col := range columns {
				columnNames[i] = fmt.Sprintf("\"%s\"", col)
			}
			columnPart := fmt.Sprintf("INSERT INTO \"%s\" (%s) VALUES ", tableName, strings.Join(columnNames, ", "))
			for rows.Next() {
				values := make([]interface{}, len(columns))
				valuePtrs := make([]interface{}, len(columns))
				for i := range columns {
					valuePtrs[i] = &values[i]
				}
				if err := rows.Scan(valuePtrs...); err != nil {
					helpers.AppLogger.Warnf("读取表%s数据失败: %v", tableName, err)
					continue
				}
				valueParts := make([]string, len(values))
				for i, val := range values {
					if val == nil {
						valueParts[i] = "NULL"
						continue
					}
					if b, ok := val.([]byte); ok {
						escapedStr := strings.ReplaceAll(string(b), "'", "''")
						valueParts[i] = fmt.Sprintf("'%s'", escapedStr)
						continue
					}
					if t, ok := val.(time.Time); ok {
						valueParts[i] = fmt.Sprintf("'%s'", t.Format(time.RFC3339Nano))
						continue
					}
					valStr := fmt.Sprintf("%v", val)
					if _, err := fmt.Sscanf(valStr, "%f", new(float64)); err == nil {
						valueParts[i] = valStr
						continue
					}
					escapedStr := strings.ReplaceAll(valStr, "'", "''")
					valueParts[i] = fmt.Sprintf("'%s'", escapedStr)
				}
				insertStmt := fmt.Sprintf("%s(%s);\n", columnPart, strings.Join(valueParts, ", "))
				file.WriteString(insertStmt)
			}
		}
		rows.Close()
		file.WriteString("\n")
	}

	helpers.AppLogger.Infof("数据库已导出到: %s (表数: %d, 数据库大小: %d bytes)", filePath, tableCount, dbSize)
	return tableCount, dbSize, nil
}

// ImportDatabaseFromSQL restores database from SQL file.
func ImportDatabaseFromSQL(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("备份文件不存在: %s", filePath)
	}
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开备份文件失败: %v", err)
	}
	defer file.Close()

	var reader io.Reader = file
	if filepath.Ext(filePath) == ".gz" {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("打开压缩文件失败: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	content := make([]byte, 0)
	buffer := make([]byte, 1024*1024)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			content = append(content, buffer[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取备份文件失败: %v", err)
		}
	}

	sqlDB, err := db.Db.DB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %v", err)
	}

	if _, err := sqlDB.Exec("SET session_replication_role = replica"); err != nil {
		helpers.AppLogger.Warnf("禁用外键约束失败: %v", err)
	}
	if _, err := sqlDB.Exec(string(content)); err != nil {
		sqlDB.Exec("SET session_replication_role = default")
		return fmt.Errorf("执行恢复SQL失败: %v", err)
	}
	if _, err := sqlDB.Exec("SET session_replication_role = default"); err != nil {
		helpers.AppLogger.Warnf("恢复外键约束失败: %v", err)
	}
	helpers.AppLogger.Infof("数据库已从备份恢复: %s", filePath)
	return nil
}

func getDatabaseType() string {
	if db.Manager != nil {
		if db.Db != nil {
			dialector := db.Db.Dialector
			switch dialector.Name() {
			case "postgres":
				return "postgres"
			case "sqlite":
				return "sqlite"
			case "mysql":
				return "mysql"
			default:
				helpers.AppLogger.Warnf("未知的数据库类型: %s，使用默认 postgres", dialector.Name())
				return "postgres"
			}
		}
	}
	return "postgres"
}

func cleanupOldBackups(config *models.BackupConfig) {
	backupDir := filepath.Join(helpers.ConfigDir, config.BackupPath)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		helpers.AppLogger.Errorf("读取备份目录失败: %v", err)
		return
	}

	var backupFiles []os.FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if filepath.Ext(filename) != ".sql" && filepath.Ext(filename) != ".gz" {
			continue
		}
		info, _ := entry.Info()
		backupFiles = append(backupFiles, info)
	}

	for i := 0; i < len(backupFiles); i++ {
		for j := i + 1; j < len(backupFiles); j++ {
			if backupFiles[i].ModTime().Before(backupFiles[j].ModTime()) {
				backupFiles[i], backupFiles[j] = backupFiles[j], backupFiles[i]
			}
		}
	}

	now := time.Now()
	maxCount := config.BackupMaxCount
	retentionDays := config.BackupRetention

	if len(backupFiles) > maxCount {
		for i := maxCount; i < len(backupFiles); i++ {
			filePath := filepath.Join(backupDir, backupFiles[i].Name())
			if err := os.Remove(filePath); err != nil {
				helpers.AppLogger.Warnf("删除备份文件失败: %s, %v", backupFiles[i].Name(), err)
			} else {
				helpers.AppLogger.Infof("已删除超期备份: %s", backupFiles[i].Name())
			}
		}
	}

	for _, fileInfo := range backupFiles {
		if now.Sub(fileInfo.ModTime()) > time.Duration(retentionDays)*24*time.Hour {
			filePath := filepath.Join(backupDir, fileInfo.Name())
			if err := os.Remove(filePath); err != nil {
				helpers.AppLogger.Warnf("删除备份文件失败: %s, %v", fileInfo.Name(), err)
			} else {
				helpers.AppLogger.Infof("已删除超期备份: %s", fileInfo.Name())
			}
		}
	}
}

func compressFile(sourceFile, targetFile string) error {
	source, err := os.Open(sourceFile)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(targetFile)
	if err != nil {
		return err
	}
	defer target.Close()

	gzipWriter := gzip.NewWriter(target)
	defer gzipWriter.Close()

	_, err = io.Copy(gzipWriter, source)
	return err
}

// resetAllSequences is kept for compatibility; currently unused.
func resetAllSequences(sqlDB *sql.DB) error {
	rows, err := sqlDB.Query(`
        SELECT table_name FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
    `)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return err
		}
		tables = append(tables, tableName)
	}

	for _, tableName := range tables {
		var pkCol string
		err := sqlDB.QueryRow(fmt.Sprintf(`
            SELECT a.attname FROM pg_index i 
            JOIN pg_attribute a ON a.attrelid = i.indrelid 
            AND a.attnum = ANY(i.indkey) 
            WHERE i.indrelid = '%s'::regclass AND i.indisprimary
        `, tableName)).Scan(&pkCol)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			helpers.AppLogger.Warnf("查询表%s主键失败: %v", tableName, err)
			continue
		}
		if pkCol == "" {
			continue
		}
		var maxId int64
		err = sqlDB.QueryRow(fmt.Sprintf(`SELECT COALESCE(MAX("%s"), 0) FROM "%s"`, pkCol, tableName)).Scan(&maxId)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			helpers.AppLogger.Warnf("查询表%s的最大ID失败: %v", tableName, err)
			continue
		}
		var seqName string
		err = sqlDB.QueryRow(fmt.Sprintf(`
            SELECT pg_get_serial_sequence('%s', '%s')
        `, tableName, pkCol)).Scan(&seqName)
		if err != nil || seqName == "" {
			continue
		}
		nextVal := maxId + 1
		if _, err := sqlDB.Exec(fmt.Sprintf(`ALTER SEQUENCE "%s" RESTART WITH %d`, seqName, nextVal)); err != nil {
			helpers.AppLogger.Warnf("重置序列%s失败: %v", seqName, err)
		} else {
			helpers.AppLogger.Infof("序列%s已重置为%d", seqName, nextVal)
		}
	}
	return nil
}
