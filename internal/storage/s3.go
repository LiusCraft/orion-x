package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// s3Storage 是 Storage 的 S3 协议实现。
type s3Storage struct {
	client *s3.Client
	bucket string
}

// newS3 构造 S3 客户端。
//
// 兼容性说明：新版 AWS SDK 默认给 PutObject 附加 CRC32 校验和与 aws-chunked
// 传输编码，七牛 Kodo 等 S3 兼容服务未实现这些字段（见七牛《兼容 API》）。
// 关掉默认校验和后，请求回到「普通 PUT + 真实 payload 哈希」。
func newS3(ctx context.Context, cfg Config) (Storage, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: load aws config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.PathStyle()
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	return &s3Storage{client: client, bucket: cfg.Bucket}, nil
}

func (c Config) validate() error {
	missing := make([]string, 0, 5)
	if c.Endpoint == "" {
		missing = append(missing, "endpoint")
	}
	if c.Region == "" {
		missing = append(missing, "region")
	}
	if c.Bucket == "" {
		missing = append(missing, "bucket")
	}
	if c.AccessKey == "" {
		missing = append(missing, "access_key")
	}
	if c.SecretKey == "" {
		missing = append(missing, "secret_key")
	}
	if len(missing) > 0 {
		return fmt.Errorf("storage: missing required config: %s", strings.Join(missing, ", "))
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("storage: endpoint must be a http(s) URL, got %q", c.Endpoint)
	}
	return nil
}

func (s *s3Storage) Put(ctx context.Context, key string, body io.Reader, opts PutOptions) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(opts.Size),
		ContentType:   aws.String(opts.ContentType),
	})
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	return nil
}

func (s *s3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage: get %s: %w", key, err)
	}
	return out.Body, nil
}

func (s *s3Storage) Delete(ctx context.Context, key string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	return nil
}

func (s *s3Storage) PresignGet(ctx context.Context, key string, ttl time.Duration, opts PresignOptions) (string, error) {
	if ttl <= 0 {
		ttl = defaultPresignTTL
	}
	in := &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	if opts.DownloadFileName != "" {
		in.ResponseContentDisposition = aws.String(attachmentDisposition(opts.DownloadFileName))
	}
	out, err := s3.NewPresignClient(s.client, s3.WithPresignExpires(ttl)).PresignGetObject(ctx, in)
	if err != nil {
		return "", fmt.Errorf("storage: presign %s: %w", key, err)
	}
	return out.URL, nil
}

func (s *s3Storage) Ping(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("storage: head bucket %s: %w", s.bucket, err)
	}
	return nil
}

// isNotFound 识别 S3 兼容服务的「对象不存在」错误：先看错误码，再退回 404 状态码。
func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return true
		}
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		return respErr.HTTPStatusCode() == http.StatusNotFound
	}
	return false
}

// attachmentDisposition 构造 content-disposition 值。
// 文件名里的引号与控制字符会被去掉，避免响应头注入。
func attachmentDisposition(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" {
		return "attachment"
	}
	if isASCII(name) {
		return "attachment; filename=\"" + name + "\""
	}
	return "attachment; filename*=UTF-8''" + url.PathEscape(name)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
