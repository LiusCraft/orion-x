// Package storage 提供对象存储的读写能力。
//
// 当前只有一个实现：s3.go（AWS S3 协议），可对接 AWS S3、七牛云 Kodo、MinIO、
// 阿里云 OSS 等任何 S3 兼容服务——换厂商只改配置，不引入厂商 SDK。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Storage 是对象存储的最小能力集。
type Storage interface {
	// Put 写入对象。body 会被完整读取；opts.Size 必须给出（S3 普通 PUT 需要 Content-Length）。
	Put(ctx context.Context, key string, body io.Reader, opts PutOptions) error
	// Get 读取对象；对象不存在时返回 ErrNotFound。
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete 删除对象；对象不存在视为成功（S3 语义，便于重试幂等）。
	Delete(ctx context.Context, key string) error
	// PresignGet 返回带签名的临时读取 URL。纯本地计算，不产生网络请求。
	PresignGet(ctx context.Context, key string, ttl time.Duration, opts PresignOptions) (string, error)
	// Ping 检查桶是否可用（HeadBucket），用于启动自检。
	Ping(ctx context.Context) error
}

// PutOptions 描述一次写入。
type PutOptions struct {
	ContentType string
	Size        int64
}

// PresignOptions 描述预签名 URL 的附加行为。
type PresignOptions struct {
	// DownloadFileName 非空时附带 response-content-disposition: attachment，
	// 访问该 URL 时按此文件名下载（七牛：仅签名请求支持自定义响应头）。
	DownloadFileName string
}

// ErrNotFound 表示对象不存在。
var ErrNotFound = errors.New("storage: object not found")

// TypeS3 是当前唯一支持的存储类型（S3 协议）。
const TypeS3 = "s3"

const (
	defaultPresignTTL    = 15 * time.Minute
	defaultMaxUploadSize = 50 << 20
)

// Config 是对象存储配置，同时用于 manager.yaml 的 storage 段。
type Config struct {
	Type      string `yaml:"type"`     // 目前仅 "s3"
	Endpoint  string `yaml:"endpoint"` // 如 https://s3.cn-east-1.qiniucs.com
	Region    string `yaml:"region"`   // 必须与 endpoint 的区域一致（参与 SigV4 签名）
	Bucket    string `yaml:"bucket"`   // 七牛填「S3 空间名」
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	// UsePathStyle 默认 true（Kodo / MinIO 建议）；需要 virtual-host 寻址时显式设为 false。
	UsePathStyle  *bool         `yaml:"use_path_style"`
	Prefix        string        `yaml:"prefix"`          // 可选 key 前缀，多环境共桶时用 dev/prod
	PresignTTL    time.Duration `yaml:"presign_ttl"`     // 预签名有效期，默认 15m
	MaxUploadSize int64         `yaml:"max_upload_size"` // 全局单文件上限，默认 50MB
}

// Enabled 报告是否配置了对象存储。
func (c Config) Enabled() bool { return strings.TrimSpace(c.Type) != "" }

// PathStyle 报告是否使用 path-style 寻址。
func (c Config) PathStyle() bool { return c.UsePathStyle == nil || *c.UsePathStyle }

// WithDefaults 返回补全默认值并归一化后的配置。
func (c Config) WithDefaults() Config {
	c.Endpoint = strings.TrimRight(strings.TrimSpace(c.Endpoint), "/")
	c.Prefix = strings.Trim(strings.TrimSpace(c.Prefix), "/")
	if c.PresignTTL <= 0 {
		c.PresignTTL = defaultPresignTTL
	}
	if c.MaxUploadSize <= 0 {
		c.MaxUploadSize = defaultMaxUploadSize
	}
	return c
}

// New 根据 Config.Type 构造实现。
func New(ctx context.Context, cfg Config) (Storage, error) {
	cfg = cfg.WithDefaults()
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "":
		return nil, errors.New("storage: type is required")
	case TypeS3:
		return newS3(ctx, cfg)
	default:
		return nil, fmt.Errorf("storage: unsupported type %q (supported: %s)", cfg.Type, TypeS3)
	}
}
