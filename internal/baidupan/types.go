package baidupan

import (
	"time"
)

// RespBase 基础响应结构
type RespBase[T any] struct {
	State   int    `json:"state"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// RespBaseBool 基础响应结构（布尔状态）
type RespBaseBool[T any] struct {
	State   bool   `json:"state"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// RequestConfig 请求配置
type RequestConfig struct {
	MaxRetries      int           `json:"max_retries"`
	RetryDelay      time.Duration `json:"retry_delay"`
	Timeout         time.Duration `json:"timeout"`
	BypassRateLimit bool          `json:"bypass_rate_limit"` // 是否绕过速率限制
}

// DefaultRequestConfig 默认请求配置
func DefaultRequestConfig() *RequestConfig {
	return &RequestConfig{
		MaxRetries: DEFAULT_MAX_RETRIES,
		RetryDelay: DEFAULT_RETRY_DELAY * time.Second,
		Timeout:    DEFAULT_TIMEOUT * time.Second,
	}
}

func MakeRequestConfig(maxRetries int, retryDelay time.Duration, timeout time.Duration) *RequestConfig {
	config := DefaultRequestConfig()
	if maxRetries > 0 {
		config.MaxRetries = maxRetries
	}
	if retryDelay > 0 {
		config.RetryDelay = retryDelay * time.Second
	}
	if timeout > 0 {
		config.Timeout = timeout * time.Second
	}
	return config
}

// RateLimitConfig 限速配置
type RateLimitConfig struct {
	QPSLimit int64 // 每秒请求数限制
	QPMLimit int64 // 每分钟请求数限制
	QPHLimit int64 // 每小时请求数限制
	QPTLimit int64 // 每天请求数限制
}

// DefaultRateLimitConfig 默认限速配置
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		QPSLimit: DEFAULT_QPS_LIMIT,
		QPMLimit: DEFAULT_QPM_LIMIT,
		QPHLimit: DEFAULT_QPH_LIMIT,
		QPTLimit: DEFAULT_QPT_LIMIT,
	}
}

// UserInfoResponse 用户信息响应
type UserInfoResponse struct {
	AvatarUrl   string `json:"avatar_url"`
	CreateTime  int64  `json:"create_time"`
	Email       string `json:"email"`
	Level       int    `json:"level"`
	Nickname    string `json:"nickname"`
	Quota       int64  `json:"quota"`        // 总空间(字节)
	Used        int64  `json:"used"`         // 已用空间(字节)
	Username    string `json:"username"`
	VipType     int    `json:"vip_type"`
	NetDiskName string `json:"netdisk_name"`
	UserId      string `json:"uk"`
}

// FileInfo 文件信息
type FileInfo struct {
	Ctime        int64  `json:"ctime"`         // 创建时间
	FsID         int64  `json:"fs_id"`         // 文件系统ID
	IsDir        int    `json:"isdir"`         // 是否为目录
	List         int    `json:"list"`          // 子目录数量
	Mtime        int64  `json:"mtime"`         // 修改时间
	Path         string `json:"path"`          // 路径
	ServerCtime  int64  `json:"server_ctime"`  // 服务器创建时间
	ServerMtime  int64  `json:"server_mtime"`  // 服务器修改时间
	Size         int64  `json:"size"`          // 文件大小(字节)
	Category     int    `json:"category"`      // 文件类别
	OwnerType    int    `json:"owner_type"`    // 所有者类型
	Thumbs       map[string]string `json:"thumbs"`
	OperID       int64  `json:"oper_id"`       // 操作者ID
	Share        int    `json:"share"`         // 共享状态
	Extra        string `json:"extra"`         // 扩展信息
	Md5          string `json:"md5"`           // 文件MD5
}

// FileListResponse 文件列表响应
type FileListResponse struct {
	HasMore   int        `json:"has_more"`   // 是否还有更多
	List      []FileInfo `json:"list"`       // 文件列表
	PageNum   int        `json:"page_num"`   // 当前页码
	PageTotal int        `json:"page_total"` // 总页数
}

// UploadResponse 上传响应
type UploadResponse struct {
	Path  string `json:"path"` // 文件路径
	Size  int64  `json:"size"` // 文件大小
	FSID  int64  `json:"fs_id"` // 文件系统ID
	Mtime int64  `json:"mtime"` // 修改时间
	Ctime int64  `json:"ctime"` // 创建时间
	MD5   string `json:"md5"`   // 文件MD5
}

// PrecreateResponse 预创建响应
type PrecreateResponse struct {
	Path      string `json:"path"` // 上传路径
	UploadID  string `json:"uploadid"` // 上传ID
	BlockList []struct {
		MD5 string `json:"md5"`
	} `json:"block_list"`
	ReturnType int `json:"return_type"` // 返回类型
}

// UploadSliceResponse 分片上传响应
type UploadSliceResponse struct {
	MD5      string `json:"md5"`       // 分片MD5
	Offset   int64  `json:"offset"`    // 偏移量
	SliceMD5 string `json:"slice_md5"` // 分片MD5
}

// CreateFileResponse 创建文件响应
type CreateFileResponse struct {
	Path  string `json:"path"` // 文件路径
	FSID  int64  `json:"fs_id"` // 文件系统ID
	Size  int64  `json:"size"` // 文件大小
	Ctime int64  `json:"ctime"` // 创建时间
	Mtime int64  `json:"mtime"` // 修改时间
	MD5   string `json:"md5"`   // 文件MD5
}

// FileManagerResponse 文件管理响应
type FileManagerResponse struct {
	Extra struct {
		List []struct {
			To   string `json:"to"`
			From string `json:"from"`
		} `json:"list"`
	} `json:"extra"`
	List []FileInfo `json:"list"`
}

// DownloadResponse 下载响应
type DownloadResponse struct {
	DownloadURL string `json:"download_url"` // 下载链接
	ExpireTime  int64  `json:"expire_time"`  // 过期时间
}
