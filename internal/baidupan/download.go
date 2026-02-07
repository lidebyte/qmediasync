package baidupan

import (
	"Q115-STRM/internal/helpers"
	"context"
	"fmt"
)

// FileDownload 文件下载
func (c *BaiDuPanClient) FileDownload(ctx context.Context, path string) (*DownloadResponse, error) {
	url := fmt.Sprintf("%s/rest/2.0/xpan/file?method=download&access_token=%s&path=%s", API_BASE_URL, c.AccessToken, path)
	req := c.client.R()

	var downloadResp DownloadResponse
	response, _, err := c.request(url, req)
	if err != nil {
		helpers.AppLogger.Errorf("文件下载失败: %v", err)
		return nil, fmt.Errorf("文件下载失败: %v", err)
	}

	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("文件下载失败: HTTP %d", response.StatusCode())
	}

	helpers.AppLogger.Infof("成功获取文件下载链接: path=%s", path)
	return &downloadResp, nil
}

// GetDownloadURL 获取文件下载直链
func (c *BaiDuPanClient) GetDownloadURL(ctx context.Context, path string) (string, error) {
	downloadResp, err := c.FileDownload(ctx, path)
	if err != nil {
		return "", err
	}

	return downloadResp.DownloadURL, nil
}
