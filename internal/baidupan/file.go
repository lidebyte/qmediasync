package baidupan

import (
	"Q115-STRM/internal/helpers"
	"context"
	"fmt"
)

// FileList 查询文件列表
func (c *BaiDuPanClient) FileList(ctx context.Context, dir string, page int, pageSize int) (*FileListResponse, error) {
	url := fmt.Sprintf("%s/rest/2.0/xpan/file?method=list&access_token=%s&dir=%s&page=%d&num=%d", API_BASE_URL, c.AccessToken, dir, page, pageSize)
	req := c.client.R()

	var fileList FileListResponse
	response, _, err := c.request(url, req)
	if err != nil {
		helpers.AppLogger.Errorf("获取文件列表失败: %v", err)
		return nil, fmt.Errorf("获取文件列表失败: %v", err)
	}

	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("获取文件列表失败: HTTP %d", response.StatusCode())
	}

	helpers.AppLogger.Infof("成功获取文件列表: dir=%s, count=%d", dir, len(fileList.List))
	return &fileList, nil
}

// RecursiveFileList 递归获取文件列表
func (c *BaiDuPanClient) RecursiveFileList(ctx context.Context, dir string) ([]FileInfo, error) {
	var allFiles []FileInfo
	page := 1
	pageSize := 100

	for {
		fileList, err := c.FileList(ctx, dir, page, pageSize)
		if err != nil {
			return nil, err
		}

		allFiles = append(allFiles, fileList.List...)

		if fileList.HasMore == 0 {
			break
		}

		page++
	}

	for _, file := range allFiles {
		if file.IsDir == 1 {
			subFiles, err := c.RecursiveFileList(ctx, file.Path)
			if err != nil {
				helpers.AppLogger.Warnf("递归获取文件列表失败: %v", err)
				continue
			}
			allFiles = append(allFiles, subFiles...)
		}
	}

	return allFiles, nil
}

// FileInfo 查询文件详情
func (c *BaiDuPanClient) FileInfo(ctx context.Context, path string) (*FileInfo, error) {
	url := fmt.Sprintf("%s/rest/2.0/xpan/file?method=meta&access_token=%s&path=%s", API_BASE_URL, c.AccessToken, path)
	req := c.client.R()

	var fileInfo FileInfo
	response, _, err := c.request(url, req)
	if err != nil {
		helpers.AppLogger.Errorf("获取文件详情失败: %v", err)
		return nil, fmt.Errorf("获取文件详情失败: %v", err)
	}

	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("获取文件详情失败: HTTP %d", response.StatusCode())
	}

	helpers.AppLogger.Infof("成功获取文件详情: path=%s, size=%d", path, fileInfo.Size)
	return &fileInfo, nil
}

// FileManager 文件管理（支持copy、move、rename、delete）
func (c *BaiDuPanClient) FileManager(ctx context.Context, operation string, fileList []string, destPath string) (*FileManagerResponse, error) {
	url := fmt.Sprintf("%s/rest/2.0/xpan/file?method=filemanager&access_token=%s&opera=%s", API_BASE_URL, c.AccessToken, operation)

	type fileManagerReq struct {
		FileList  []string `json:"filelist"`
		Async     bool     `json:"async"`
		OnDup     string   `json:"ondup"`
		TargetDir string   `json:"target_dir"`
	}

	reqBody := fileManagerReq{
		FileList:  fileList,
		Async:     false,
		OnDup:     "overwrite",
		TargetDir: destPath,
	}

	req := c.client.R().SetBody(reqBody)

	var fileManagerResp FileManagerResponse
	response, _, err := c.request(url, req)
	if err != nil {
		helpers.AppLogger.Errorf("文件管理失败: %v", err)
		return nil, fmt.Errorf("文件管理失败: %v", err)
	}

	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("文件管理失败: HTTP %d", response.StatusCode())
	}

	helpers.AppLogger.Infof("成功执行文件管理: operation=%s, count=%d", operation, len(fileList))
	return &fileManagerResp, nil
}

// Copy 复制文件
func (c *BaiDuPanClient) Copy(ctx context.Context, fileList []string, destPath string) (*FileManagerResponse, error) {
	return c.FileManager(ctx, "copy", fileList, destPath)
}

// Move 移动文件
func (c *BaiDuPanClient) Move(ctx context.Context, fileList []string, destPath string) (*FileManagerResponse, error) {
	return c.FileManager(ctx, "move", fileList, destPath)
}

// Rename 重命名文件
func (c *BaiDuPanClient) Rename(ctx context.Context, fileList []string, destPath string) (*FileManagerResponse, error) {
	return c.FileManager(ctx, "rename", fileList, destPath)
}

// Delete 删除文件
func (c *BaiDuPanClient) Delete(ctx context.Context, fileList []string) (*FileManagerResponse, error) {
	return c.FileManager(ctx, "delete", fileList, "")
}
