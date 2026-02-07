package baidupan

import (
	"Q115-STRM/internal/helpers"
	"context"
	"fmt"
)

// Precreate 预创建文件
func (c *BaiDuPanClient) Precreate(ctx context.Context, path string, blockSize int64, md5 string) (*PrecreateResponse, error) {
	url := fmt.Sprintf("%s/rest/2.0/xpan/file?method=precreate&access_token=%s", API_BASE_URL, c.AccessToken)

	type precreateReq struct {
		Path       string `json:"path"`
		BlockSize  int64  `json:"block_size"`
		ContentMD5 string `json:"content_md5"`
	}

	reqBody := precreateReq{
		Path:       path,
		BlockSize:  blockSize,
		ContentMD5: md5,
	}

	req := c.client.R().SetBody(reqBody)

	var precreateResp PrecreateResponse
	response, _, err := c.request(url, req)
	if err != nil {
		helpers.AppLogger.Errorf("预创建文件失败: %v", err)
		return nil, fmt.Errorf("预创建文件失败: %v", err)
	}

	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("预创建文件失败: HTTP %d", response.StatusCode())
	}

	helpers.AppLogger.Infof("成功预创建文件: path=%s", path)
	return &precreateResp, nil
}

// UploadSlice 上传分片
func (c *BaiDuPanClient) UploadSlice(ctx context.Context, uploadID string, partSeq int, fileData []byte) (*UploadSliceResponse, error) {
	url := fmt.Sprintf("%s/rest/2.0/xpan/file?method=upload&access_token=%s&uploadid=%s&partseq=%d", API_BASE_URL, c.AccessToken, uploadID, partSeq)
	req := c.client.R().SetBody(fileData)

	var uploadSliceResp UploadSliceResponse
	response, _, err := c.request(url, req)
	if err != nil {
		helpers.AppLogger.Errorf("上传分片失败: %v", err)
		return nil, fmt.Errorf("上传分片失败: %v", err)
	}

	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("上传分片失败: HTTP %d", response.StatusCode())
	}

	return &uploadSliceResp, nil
}

// CreateFile 创建文件
func (c *BaiDuPanClient) CreateFile(ctx context.Context, path string, blockSize int64, blockList []string) (*CreateFileResponse, error) {
	url := fmt.Sprintf("%s/rest/2.0/xpan/file?method=create&access_token=%s", API_BASE_URL, c.AccessToken)

	type createFileReq struct {
		Path      string   `json:"path"`
		BlockSize int64    `json:"block_list"`
		BlockList []string `json:"block_list"`
	}

	reqBody := createFileReq{
		Path:      path,
		BlockSize: blockSize,
		BlockList: blockList,
	}

	req := c.client.R().SetBody(reqBody)

	var createFileResp CreateFileResponse
	response, _, err := c.request(url, req)
	if err != nil {
		helpers.AppLogger.Errorf("创建文件失败: %v", err)
		return nil, fmt.Errorf("创建文件失败: %v", err)
	}

	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("创建文件失败: HTTP %d", response.StatusCode())
	}

	helpers.AppLogger.Infof("成功创建文件: path=%s", path)
	return &createFileResp, nil
}

// SmallFileUpload 小文件上传
func (c *BaiDuPanClient) SmallFileUpload(ctx context.Context, path string, fileData []byte) (*UploadResponse, error) {
	url := fmt.Sprintf("%s/rest/2.0/xpan/file?method=upload&access_token=%s&dir=%s&ondup=overwrite", API_BASE_URL, c.AccessToken, path)
	req := c.client.R().SetBody(fileData)

	var uploadResp UploadResponse
	response, _, err := c.request(url, req)
	if err != nil {
		helpers.AppLogger.Errorf("小文件上传失败: %v", err)
		return nil, fmt.Errorf("小文件上传失败: %v", err)
	}

	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("小文件上传失败: HTTP %d", response.StatusCode())
	}

	helpers.AppLogger.Infof("成功上传小文件: path=%s, size=%d", path, len(fileData))
	return &uploadResp, nil
}

// LargeFileUpload 大文件分片上传
func (c *BaiDuPanClient) LargeFileUpload(ctx context.Context, path string, fileData []byte, blockSize int64) (*UploadResponse, error) {
	fileSize := int64(len(fileData))
	totalBlocks := fileSize / blockSize
	if fileSize%blockSize != 0 {
		totalBlocks++
	}

	helpers.AppLogger.Infof("开始上传大文件: path=%s, size=%d, blockSize=%d, totalBlocks=%d", path, fileSize, blockSize, totalBlocks)
	for i := int64(0); i < totalBlocks; i++ {
		start := i * blockSize
		end := start + blockSize
		if end > fileSize {
			end = fileSize
		}

		chunk := fileData[start:end]
		_, err := c.UploadSlice(ctx, "", int(i), chunk)
		if err != nil {
			return nil, fmt.Errorf("上传分片 %d 失败: %v", i, err)
		}

		helpers.AppLogger.Infof("上传分片 %d/%d 完成", i+1, totalBlocks)
	}
	helpers.AppLogger.Infof("大文件上传完成: path=%s", path)
	return &UploadResponse{
		Path: path,
		Size: fileSize,
	}, nil
}
