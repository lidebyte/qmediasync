package controllers

import (
	"Q115-STRM/internal/backup"
	"Q115-STRM/internal/db"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/synccron"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetBackupConfig 获取备份配置
// @Summary 获取备份配置
// @Description 获取数据库备份的配置信息，包括是否启用、Cron表达式、存储路径等
// @Tags 数据库管理
// @Accept json
// @Produce json
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /database/backup-config [get]
// @Security JwtAuth
// @Security ApiKeyAuth
func GetBackupConfig(c *gin.Context) {
	config := &models.BackupConfig{}
	result := db.Db.First(config)
	if result.Error == gorm.ErrRecordNotFound {
		// 配置不存在，返回空配置
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取备份配置成功", Data: map[string]interface{}{
			"exists": false,
		}})
		return
	}
	if result.Error != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取备份配置失败: " + result.Error.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取备份配置成功", Data: map[string]interface{}{
		"exists": true,
		"config": config,
	}})
}

// UpdateBackupConfig 更新备份配置
func UpdateBackupConfig(c *gin.Context) {
	type updateBackupConfigRequest struct {
		BackupEnabled   int    `json:"backup_enabled"`   // 是否启用自动备份
		BackupCron      string `json:"backup_cron"`      // 备份cron表达式
		BackupPath      string `json:"backup_path"`      // 备份存储路径
		BackupRetention int    `json:"backup_retention"` // 备份保留天数
		BackupMaxCount  int    `json:"backup_max_count"` // 最多保留的备份数量
		BackupCompress  int    `json:"backup_compress"`  // 是否压缩备份
	}

	var req updateBackupConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}

	// 验证cron表达式
	if req.BackupEnabled == 1 && req.BackupCron != "" {
		times := helpers.GetNextTimeByCronStr(req.BackupCron, 1)
		if len(times) == 0 {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "cron表达式格式无效", Data: nil})
			return
		}

		// 检查最小间隔是否满足1小时要求
		if !backup.ValidateCronInterval(req.BackupCron) {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "备份任务定时最小间隔为1小时", Data: nil})
			return
		}
	}

	config := &models.BackupConfig{}
	result := db.Db.First(config)
	if result.Error == gorm.ErrRecordNotFound {
		// 创建新的配置记录
		config = &models.BackupConfig{
			BackupEnabled:   req.BackupEnabled,
			BackupCron:      req.BackupCron,
			BackupPath:      req.BackupPath,
			BackupRetention: req.BackupRetention,
			BackupMaxCount:  req.BackupMaxCount,
			BackupCompress:  req.BackupCompress,
		}
		if err := db.Db.Create(config).Error; err != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "创建备份配置失败: " + err.Error(), Data: nil})
			return
		}
	} else if result.Error != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询备份配置失败: " + result.Error.Error(), Data: nil})
		return
	} else {
		// 更新现有配置
		updateData := map[string]interface{}{
			"backup_enabled":   req.BackupEnabled,
			"backup_cron":      req.BackupCron,
			"backup_path":      req.BackupPath,
			"backup_retention": req.BackupRetention,
			"backup_max_count": req.BackupMaxCount,
			"backup_compress":  req.BackupCompress,
		}
		if err := db.Db.Model(config).Updates(updateData).Error; err != nil {
			c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新备份配置失败: " + err.Error(), Data: nil})
			return
		}
	}

	// 重新初始化定时任务
	synccron.InitCron()

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "备份配置更新成功", Data: nil})
}

// StartBackupTask 启动备份任务
// @Summary 启动手动备份
// @Description 手动启动一个数据库备份任务
// @Tags 数据库管理
// @Accept json
// @Produce json
// @Param reason body string false "备份原因"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /database/backup-start [post]
// @Security JwtAuth
// @Security ApiKeyAuth
func StartBackupTask(c *gin.Context) {
	type startBackupRequest struct {
		Reason string `json:"reason"` // 备份原因
	}

	var req startBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}

	// 检查是否已有运行中的备份任务（内存）
	if models.GetCurrentBackupTask() != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "已有备份任务正在运行中，请等待完成", Data: nil})
		return
	}

	// 获取备份配置
	config := &models.BackupConfig{}
	if err := db.Db.First(config).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "未找到备份配置，请先配置", Data: nil})
		return
	}

	// 创建运行时备份任务（内存）
	task := &models.RuntimeBackupTask{
		ID:            models.NewBackupTaskID(),
		Status:        "running",
		Progress:      0,
		BackupType:    "manual",
		CreatedReason: req.Reason,
		CurrentStep:   "准备备份...",
		StartTime:     time.Now().Unix(),
	}
	models.SetCurrentBackupTask(task)

	// 异步执行备份
	go backup.PerformManualBackup(config)

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "备份任务已启动", Data: map[string]interface{}{
		"task_id": task.ID,
	}})
}

// CancelBackupTask 取消备份任务
// @Summary 取消备份任务
// @Description 取消正在运行中的备份任务
// @Tags 数据库管理
// @Accept json
// @Produce json
// @Param task_id body integer true "备份任务ID"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /database/backup-cancel [post]
// @Security JwtAuth
// @Security ApiKeyAuth
func CancelBackupTask(c *gin.Context) {
	type cancelBackupRequest struct {
		TaskID uint `json:"task_id" binding:"required"` // 任务ID
	}

	var req cancelBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}

	task := models.GetCurrentBackupTask()
	if task == nil || task.ID != req.TaskID {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "任务不存在", Data: nil})
		return
	}

	if task.Status != "running" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "只能取消运行中的任务", Data: nil})
		return
	}

	// 标记任务为已取消（内存）
	models.UpdateBackupTask(func(t *models.RuntimeBackupTask) {
		t.Status = "cancelled"
		t.EndTime = time.Now().Unix()
		t.FailureReason = "用户取消"
	})

	// 清理临时文件
	if task.FilePath != "" {
		config := &models.BackupConfig{}
		db.Db.First(config)
		if config.ID > 0 {
			backupDir := filepath.Join(helpers.ConfigDir, config.BackupPath)
			os.Remove(filepath.Join(backupDir, task.FilePath))
			os.Remove(filepath.Join(backupDir, task.FilePath+".gz"))
		}
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "备份任务已取消", Data: nil})
}

// GetBackupProgress 查询备份进度
// @Summary 查询备份进度
// @Description 查询当前进行中的备份任务的进度信息
// @Tags 数据库管理
// @Accept json
// @Produce json
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /database/backup-progress [get]
// @Security JwtAuth
// @Security ApiKeyAuth
func GetBackupProgress(c *gin.Context) {
	// 从内存获取当前备份任务
	task := models.GetCurrentBackupTask()
	if task == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "没有运行中的备份任务", Data: map[string]interface{}{
			"running": false,
		}})
		return
	}

	// 计算已耗时间
	now := time.Now().Unix()
	elapsedSeconds := now - task.StartTime
	estimatedSeconds := task.EstimatedSeconds
	if estimatedSeconds == 0 {
		estimatedSeconds = 3600 // 默认1小时
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取进度成功", Data: map[string]interface{}{
		"task_id":           task.ID,
		"running":           task.Status == "running",
		"status":            task.Status,
		"progress":          task.Progress,
		"elapsed_seconds":   elapsedSeconds,
		"estimated_seconds": estimatedSeconds,
		"current_step":      task.CurrentStep,
		"processed_tables":  task.ProcessedTables,
		"total_tables":      task.TotalTables,
	}})
}

// RestoreDatabase 恢复数据库
// @Summary 恢复数据库
// @Description 上传备份文件并恢复数据库
// @Tags 数据库管理
// @Accept multipart/form-data
// @Produce json
// @Param backup_file formData file true "备份文件（.sql或.sql.gz）"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /database/restore [post]
// @Security JwtAuth
// @Security ApiKeyAuth
func RestoreDatabase(c *gin.Context) {
	// 获取上传的备份文件
	file, err := c.FormFile("backup_file")
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "获取上传文件失败: " + err.Error(), Data: nil})
		return
	}

	// 验证文件大小（限制为1GB）
	if file.Size > 1024*1024*1024 {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "备份文件过大，最大支持1GB", Data: nil})
		return
	}

	// 验证文件扩展名
	ext := filepath.Ext(file.Filename)
	if ext != ".sql" && ext != ".gz" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "只支持.sql或.sql.gz格式的备份文件", Data: nil})
		return
	}

	// 检查是否已有运行中的备份任务
	if models.GetCurrentBackupTask() != nil && models.GetCurrentBackupTask().Status == "running" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "有备份任务正在运行中，无法进行恢复", Data: nil})
		return
	}

	// 检查是否已有运行中的恢复任务
	if models.GetCurrentRestoreTask() != nil && models.GetCurrentRestoreTask().Status == "running" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "已有恢复任务正在运行中，请等待完成", Data: nil})
		return
	}

	// 保存临时文件
	tempDir := filepath.Join(helpers.ConfigDir, "backups", "temp")
	os.MkdirAll(tempDir, 0755)
	tempFilePath := filepath.Join(tempDir, file.Filename)

	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "保存上传文件失败: " + err.Error(), Data: nil})
		return
	}

	// 验证备份文件完整性
	if !backup.ValidateBackupFile(tempFilePath) {
		os.Remove(tempFilePath)
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "备份文件格式无效或已损坏", Data: nil})
		return
	}

	// 创建内存中的恢复任务
	task := &models.RuntimeRestoreTask{
		ID:               models.NewRestoreTaskID(),
		Status:           "running",
		Progress:         0,
		CurrentStep:      "准备恢复...",
		SourceFile:       file.Filename,
		StartTime:        time.Now().Unix(),
		EstimatedSeconds: 3600,
	}
	models.SetCurrentRestoreTask(task)

	// 构造恢复流程的钩子
	hooks := &backup.RestoreHooks{
		StopCron: func() {
			if synccron.GlobalCron != nil {
				synccron.GlobalCron.Stop()
			}
		},
		StartCron: func() {
			synccron.InitCron()
			synccron.ResumeAllNewSyncQueues()
		},
		PauseSyncQueue:  func() { synccron.PauseAllNewSyncQueues() },
		ResumeSyncQueue: func() { synccron.ResumeAllNewSyncQueues() },
		StopUploadQueue: func() {
			if models.GlobalUploadQueue != nil {
				models.GlobalUploadQueue.Stop()
			}
		},
		ResumeUploadQueue: func() {
			if models.GlobalUploadQueue != nil {
				models.GlobalUploadQueue.Restart()
			}
		},
		StopDownloadQueue: func() {
			if models.GlobalDownloadQueue != nil {
				models.GlobalDownloadQueue.Stop()
			}
		},
		ResumeDownloadQueue: func() {
			if models.GlobalDownloadQueue != nil {
				models.GlobalDownloadQueue.Restart()
			}
		},
	}

	// 异步执行恢复
	go backup.PerformRestore(tempFilePath, hooks)

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "恢复任务已启动", Data: map[string]interface{}{
		"task_id": task.ID,
	}})
}

// ListBackups 列出所有备份文件
// @Summary 列出备份文件列表
// @Description 获取所有已保存的备份文件及其信息
// @Tags 数据库管理
// @Accept json
// @Produce json
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /database/backups [get]
// @Security JwtAuth
// @Security ApiKeyAuth
func ListBackups(c *gin.Context) {
	config := &models.BackupConfig{}
	if err := db.Db.First(config).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "未找到备份配置", Data: nil})
		return
	}

	backupDir := filepath.Join(helpers.ConfigDir, config.BackupPath)
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取备份列表成功", Data: []interface{}{}})
		return
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "读取备份目录失败: " + err.Error(), Data: nil})
		return
	}

	var backups []map[string]interface{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		// 跳过临时文件
		if filepath.Ext(filename) != ".sql" && filepath.Ext(filename) != ".gz" {
			continue
		}

		info, _ := entry.Info()
		fileSize := info.Size()
		modTime := info.ModTime().Unix()

		// 从数据库查询备份记录
		record := &models.BackupRecord{}
		db.Db.Where("file_path = ?", filename).First(record)

		backup := map[string]interface{}{
			"filename":       filename,
			"file_size":      fileSize,
			"modified_time":  modTime,
			"backup_type":    record.BackupType,
			"created_reason": record.CreatedReason,
			"table_count":    record.TableCount,
			"database_size":  record.DatabaseSize,
		}
		backups = append(backups, backup)
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取备份列表成功", Data: backups})
}

// DeleteBackup 删除单个备份文件
// @Summary 删除备份文件
// @Description 删除指定的备份文件
// @Tags 数据库管理
// @Accept json
// @Produce json
// @Param filename query string true "备份文件名"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /database/backup-delete [delete]
// @Security JwtAuth
// @Security ApiKeyAuth
func DeleteBackup(c *gin.Context) {
	filename := c.Query("filename")
	if filename == "" {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "filename参数不能为空", Data: nil})
		return
	}

	config := &models.BackupConfig{}
	if err := db.Db.First(config).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "未找到备份配置", Data: nil})
		return
	}

	backupPath := filepath.Join(helpers.ConfigDir, config.BackupPath, filename)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "备份文件不存在", Data: nil})
		return
	}

	if err := os.Remove(backupPath); err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "删除备份文件失败: " + err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "备份文件已删除", Data: nil})
}

// DeleteBackupRecord 删除备份记录（同时删除对应的备份文件）
// @Summary 删除备份记录
// @Description 删除备份历史记录及对应的备份文件
// @Tags 数据库管理
// @Accept json
// @Produce json
// @Param record_id body integer true "备份记录ID"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /database/backup-record [delete]
// @Security JwtAuth
// @Security ApiKeyAuth
func DeleteBackupRecord(c *gin.Context) {
	type deleteBackupRecordRequest struct {
		RecordID uint `json:"record_id" binding:"required"` // 记录ID
	}

	var req deleteBackupRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "请求参数错误: " + err.Error(), Data: nil})
		return
	}

	record := &models.BackupRecord{}
	if err := db.Db.First(record, req.RecordID).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "备份记录不存在", Data: nil})
		return
	}

	config := &models.BackupConfig{}
	db.Db.First(config)

	// 删除备份文件
	backupPath := filepath.Join(helpers.ConfigDir, config.BackupPath, record.FilePath)
	os.Remove(backupPath)
	os.Remove(backupPath + ".gz")

	// 删除数据库记录
	if err := db.Db.Delete(record).Error; err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "删除备份记录失败: " + err.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "备份记录及相关文件已删除", Data: nil})
}

// GetBackupRecords 获取备份历史记录
// @Summary 获取备份历史记录
// @Description 分页获取数据库备份的历史记录
// @Tags 数据库管理
// @Accept json
// @Produce json
// @Param page query integer false "页码（默认1）"
// @Param page_size query integer false "每页数量（默认10）"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /database/backup-records [get]
// @Security JwtAuth
// @Security ApiKeyAuth
func GetBackupRecords(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("page_size", "10")

	var count int64
	var records []*models.BackupRecord

	result := db.Db.Model(&models.BackupRecord{}).Count(&count)
	if result.Error != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询记录数失败: " + result.Error.Error(), Data: nil})
		return
	}

	result = db.Db.Order("created_at DESC").Offset((toInt(page) - 1) * toInt(pageSize)).Limit(toInt(pageSize)).Find(&records)
	if result.Error != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "查询备份记录失败: " + result.Error.Error(), Data: nil})
		return
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取备份记录成功", Data: map[string]interface{}{
		"total":   count,
		"records": records,
	}})
}

// GetRestoreProgress 查询恢复进度
// @Summary 查询恢复进度
// @Description 查询当前进行中的数据库恢复任务的进度信息
// @Tags 数据库管理
// @Accept json
// @Produce json
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /database/restore-progress [get]
// @Security JwtAuth
// @Security ApiKeyAuth
func GetRestoreProgress(c *gin.Context) {
	// 从内存中读取恢复任务状态
	task := models.GetCurrentRestoreTask()

	if task == nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "没有运行中的恢复任务", Data: map[string]interface{}{
			"running": false,
		}})
		return
	}

	// 计算已耗时间
	now := time.Now().Unix()
	elapsedSeconds := now - task.StartTime
	estimatedSeconds := task.EstimatedSeconds
	if estimatedSeconds == 0 {
		estimatedSeconds = 3600 // 默认1小时
	}

	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "获取恢复进度成功", Data: map[string]interface{}{
		"task_id":           task.ID,
		"running":           task.Status == "running",
		"status":            task.Status,
		"progress":          task.Progress,
		"elapsed_seconds":   elapsedSeconds,
		"estimated_seconds": estimatedSeconds,
		"current_step":      task.CurrentStep,
		"source_file":       task.SourceFile,
		"rollback_file":     task.RollbackFile,
	}})
}

func toInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}
