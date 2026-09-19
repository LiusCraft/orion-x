// Package assets 管理「资源」（用户上传的文件）：校验、落对象存储、元数据与访问 URL。
//
// 资源由使用它的领域创建：知识库文档（kb_document）、音色参考音频（voice_sample）、
// 图片（image）。领域表只保存 asset_id 指针；对象键与预签名 URL 都从这里产生。
package assets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/knowledge/parser"
	"github.com/liuscraft/orion-x/internal/storage"
	"github.com/liuscraft/orion-x/internal/store"
)

// Purpose 资源用途，决定上传校验规则与消费方。
type Purpose string

const (
	// PurposeImage 图片：资源库直接上传（logo / 封面 / 头像）。
	PurposeImage Purpose = "image"
	// PurposeVoiceSample 音色复刻参考音频。
	PurposeVoiceSample Purpose = "voice_sample"
	// PurposeKBDocument 知识库原始文档（用于解析与向量化）。
	PurposeKBDocument Purpose = "kb_document"
)

var (
	// ErrForbidden 资源不属于当前用户，或该用途的资源不在此处删除。
	ErrForbidden = errors.New("assets: forbidden")
	// ErrNotFound 资源不存在。
	ErrNotFound = errors.New("assets: not found")
	// ErrStorage 对象存储操作失败。
	ErrStorage = errors.New("assets: storage failure")
)

// ValidationError 描述上传校验失败的原因。消息面向使用者，可直接返回前端。
type ValidationError struct{ Reason string }

// Error 实现 error。
func (e *ValidationError) Error() string { return e.Reason }

func invalid(format string, args ...any) error {
	return &ValidationError{Reason: fmt.Sprintf(format, args...)}
}

const (
	sniffLen        = 512 // 识别 Content-Type 与文件头时读取的字节数
	defaultPageSize = 20
	maxPageSize     = 100
	maxNameRunes    = 200 // Name 列 varchar(256)，按字符截断
	imageMaxSize    = 5 << 20
	voiceMaxSize    = 20 << 20
	documentMaxSize = 50 << 20
)

// spec 一个用途的上传约束。
type spec struct {
	label   string
	maxSize int64
	exts    map[string]struct{}
}

// magicSig 文件头特征，用于拦截「改名伪装」（例如 .exe 改成 .png）。
type magicSig struct {
	offset int
	prefix []byte
}

// magicByExt 按扩展名列出允许的文件头。
// 文本类扩展名不在此表中：纯文本没有可靠特征，只校验扩展名。
var magicByExt = map[string][]magicSig{
	".png":  {{prefix: []byte("\x89PNG\r\n\x1a\n")}},
	".jpg":  {{prefix: []byte("\xff\xd8\xff")}},
	".jpeg": {{prefix: []byte("\xff\xd8\xff")}},
	".gif":  {{prefix: []byte("GIF8")}},
	".webp": {{offset: 8, prefix: []byte("WEBP")}},
	".wav":  {{offset: 8, prefix: []byte("WAVE")}},
	".flac": {{prefix: []byte("fLaC")}},
	".m4a":  {{offset: 4, prefix: []byte("ftyp")}},
	".mp3":  {{prefix: []byte("ID3")}, {prefix: []byte("\xff\xfb")}, {prefix: []byte("\xff\xf3")}, {prefix: []byte("\xff\xf2")}},
}

// purposeSpecs 是上传校验规则的单一事实来源。
var purposeSpecs = buildSpecs()

func buildSpecs() map[Purpose]spec {
	// 知识库文档白名单与解析器注册表保持一致，避免「上传成功但解析不出文本」。
	kbExts := make(map[string]struct{})
	for _, ext := range parser.DefaultRegistry().SupportedExtensions() {
		if ext == ".url" { // .url 是 URL 解析器的虚拟扩展名，对应 source=url，不是可上传的文件
			continue
		}
		kbExts[ext] = struct{}{}
	}
	return map[Purpose]spec{
		PurposeImage: {
			label:   "图片",
			maxSize: imageMaxSize,
			exts:    extSet(".png", ".jpg", ".jpeg", ".webp", ".gif"),
		},
		PurposeVoiceSample: {
			label:   "音色参考音频",
			maxSize: voiceMaxSize,
			exts:    extSet(".wav", ".mp3", ".m4a", ".flac"),
		},
		PurposeKBDocument: {
			label:   "知识库文档",
			maxSize: documentMaxSize,
			exts:    kbExts,
		},
	}
}

func extSet(exts ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(exts))
	for _, ext := range exts {
		set[strings.ToLower(ext)] = struct{}{}
	}
	return set
}

// assetStore 是 Service 需要的持久化能力（*store.AssetStore 实现，测试可替换）。
type assetStore interface {
	Create(a *store.Asset) error
	GetByID(id string) (*store.Asset, error)
	List(ownerID, purpose, nameQuery string, offset, limit int) ([]store.Asset, int64, error)
	DeleteByID(id string) error
}

// Service 资源服务：校验 + 对象存储 + 元数据。
type Service struct {
	store         assetStore
	backend       storage.Storage
	prefix        string
	presignTTL    time.Duration
	maxUploadSize int64
}

// NewService 构造资源服务；prefix / 预签名有效期 / 上传上限来自 storage.Config。
func NewService(st assetStore, backend storage.Storage, cfg storage.Config) *Service {
	cfg = cfg.WithDefaults()
	return &Service{
		store:         st,
		backend:       backend,
		prefix:        cfg.Prefix,
		presignTTL:    cfg.PresignTTL,
		maxUploadSize: cfg.MaxUploadSize,
	}
}

// MaxUploadSize 返回全局单文件上限，HTTP 层用它限制请求体大小。
func (s *Service) MaxUploadSize() int64 { return s.maxUploadSize }

// Upload 一次上传的入参。Body 必须可 Seek（魔数探测后要回到起点）。
type Upload struct {
	OwnerID  string
	Purpose  Purpose
	FileName string
	Size     int64
	Body     io.Reader
}

// Create 校验并保存一个资源：先写对象、再落元数据，任一步失败都不会留下
// 「有记录但对象不存在」的资源。
func (s *Service) Create(ctx context.Context, up Upload) (*store.Asset, error) {
	sp, ok := purposeSpecs[up.Purpose]
	if !ok {
		return nil, invalid("不支持的用途 %q", up.Purpose)
	}
	if up.OwnerID == "" {
		return nil, invalid("缺少资源归属用户")
	}
	if up.Size <= 0 {
		return nil, invalid("文件内容为空")
	}
	if up.Size > sp.maxSize {
		return nil, invalid("%s 单个文件最大 %s，当前 %s", sp.label, humanSize(sp.maxSize), humanSize(up.Size))
	}
	ext := strings.ToLower(path.Ext(up.FileName))
	if _, ok := sp.exts[ext]; !ok {
		return nil, invalid("%s 仅支持 %s 格式", sp.label, strings.Join(sortedExts(sp.exts), "/"))
	}

	header, err := peekHeader(up.Body, sniffLen)
	if err != nil {
		return nil, invalid("读取文件内容失败")
	}
	if sigs := magicByExt[ext]; len(sigs) > 0 && !matchMagic(header, sigs) {
		return nil, invalid("文件内容与扩展名 %s 不符", ext)
	}

	asset := &store.Asset{
		ID:        uuid.NewString(),
		OwnerID:   up.OwnerID,
		Purpose:   string(up.Purpose),
		Name:      sanitizeName(up.FileName),
		MimeType:  detectMimeType(header, ext),
		Size:      up.Size,
		BaseModel: store.BaseModel{Creator: up.OwnerID},
	}
	asset.ObjectKey = s.objectKey(up.Purpose, asset.OwnerID, asset.ID, ext)

	if err := s.backend.Put(ctx, asset.ObjectKey, up.Body, storage.PutOptions{
		ContentType: asset.MimeType,
		Size:        up.Size,
	}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	if err := s.store.Create(asset); err != nil {
		// 元数据写入失败：尽力清掉刚写的对象，避免留下不可见的孤儿对象。
		_ = s.backend.Delete(context.WithoutCancel(ctx), asset.ObjectKey)
		return nil, fmt.Errorf("assets: save record: %w", err)
	}
	return asset, nil
}

// Get 按 ID 取资源并校验归属。
func (s *Service) Get(_ context.Context, ownerID, assetID string) (*store.Asset, error) {
	a, err := s.store.GetByID(assetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("assets: get: %w", err)
	}
	if a.OwnerID != ownerID {
		return nil, ErrForbidden
	}
	return a, nil
}

// Delete 删除资源。只允许删除资源库直接上传的图片：其他用途的资源由领域页面级联删除，
// 避免在这里删掉后领域数据失配。
func (s *Service) Delete(ctx context.Context, ownerID, assetID string) error {
	a, err := s.Get(ctx, ownerID, assetID)
	if err != nil {
		return err
	}
	if Purpose(a.Purpose) != PurposeImage {
		return ErrForbidden
	}
	return s.remove(ctx, a)
}

// DeleteCascade 级联删除领域资源（如音色删除时清掉参考音频）：校验归属与用途后
// 删除对象与记录。记录已不存在视为成功，便于领域侧重试；用途不匹配返回 ErrForbidden。
func (s *Service) DeleteCascade(ctx context.Context, ownerID, assetID string, purpose Purpose) error {
	a, err := s.Get(ctx, ownerID, assetID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if Purpose(a.Purpose) != purpose {
		return ErrForbidden
	}
	return s.remove(ctx, a)
}

func (s *Service) remove(ctx context.Context, a *store.Asset) error {
	if err := s.backend.Delete(ctx, a.ObjectKey); err != nil {
		return fmt.Errorf("%w: %v", ErrStorage, err)
	}
	if err := s.store.DeleteByID(a.ID); err != nil {
		return fmt.Errorf("assets: delete record: %w", err)
	}
	return nil
}

// ListQuery 浏览条件。Page 从 1 开始。
type ListQuery struct {
	OwnerID  string
	Purpose  Purpose // 空 = 全部用途
	Name     string  // 文件名模糊匹配
	Page     int
	PageSize int
}

// Normalize 补全分页默认值并限制上限。
func (q ListQuery) Normalize() ListQuery {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = defaultPageSize
	}
	if q.PageSize > maxPageSize {
		q.PageSize = maxPageSize
	}
	q.Name = strings.TrimSpace(q.Name)
	return q
}

// List 返回当前用户的分页资源列表与总数。
func (s *Service) List(ctx context.Context, q ListQuery) ([]store.Asset, int64, error) {
	q = q.Normalize()
	purpose := ""
	if q.Purpose != "" {
		if _, ok := purposeSpecs[q.Purpose]; !ok {
			return nil, 0, invalid("不支持的用途 %q", q.Purpose)
		}
		purpose = string(q.Purpose)
	}
	list, total, err := s.store.List(q.OwnerID, purpose, q.Name, (q.Page-1)*q.PageSize, q.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("assets: list: %w", err)
	}
	return list, total, nil
}

// PresignURL 生成临时访问 URL（纯本地计算）。download=true 时附带下载文件名。
func (s *Service) PresignURL(ctx context.Context, a *store.Asset, download bool) (string, time.Duration, error) {
	url, err := s.PresignURLWithTTL(ctx, a, s.presignTTL, download)
	if err != nil {
		return "", 0, err
	}
	return url, s.presignTTL, nil
}

// PresignURLWithTTL 用指定有效期生成临时访问 URL。消费方若不能立即取件
// （例如厂商异步拉取参考音频），需要比默认 presign_ttl 更长的窗口。
func (s *Service) PresignURLWithTTL(ctx context.Context, a *store.Asset, ttl time.Duration, download bool) (string, error) {
	opts := storage.PresignOptions{}
	if download {
		opts.DownloadFileName = a.Name
	}
	url, err := s.backend.PresignGet(ctx, a.ObjectKey, ttl, opts)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrStorage, err)
	}
	return url, nil
}

// View 是资源对外的 JSON 形态：元数据 + 临时访问 URL。
type View struct {
	store.Asset
	URL string `json:"url,omitempty"`
}

// View 给单个资源附上预签名 URL。
func (s *Service) View(ctx context.Context, a *store.Asset) (*View, error) {
	url, _, err := s.PresignURL(ctx, a, false)
	if err != nil {
		return nil, err
	}
	return &View{Asset: *a, URL: url}, nil
}

// Views 批量附预签名 URL（预签名为本地计算，逐条生成无网络开销）。
func (s *Service) Views(ctx context.Context, list []store.Asset) ([]View, error) {
	views := make([]View, 0, len(list))
	for i := range list {
		v, err := s.View(ctx, &list[i])
		if err != nil {
			return nil, err
		}
		views = append(views, *v)
	}
	return views, nil
}

// objectKey 生成存储键：{prefix}/{purpose}/{owner}/{assetID}{ext}。
// 键里不含用户输入的文件名，避免路径穿越与怪异字符。
func (s *Service) objectKey(p Purpose, ownerID, assetID, ext string) string {
	parts := make([]string, 0, 4)
	if s.prefix != "" {
		parts = append(parts, s.prefix)
	}
	return strings.Join(append(parts, string(p), ownerID, assetID+ext), "/")
}

// peekHeader 读取前 n 字节并把 reader 归位。
func peekHeader(r io.Reader, n int) ([]byte, error) {
	seeker, ok := r.(io.Seeker)
	if !ok {
		return nil, errors.New("assets: upload body must be seekable")
	}
	buf := make([]byte, n)
	read, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return buf[:read], nil
}

func matchMagic(header []byte, sigs []magicSig) bool {
	for _, sig := range sigs {
		if len(header) >= sig.offset+len(sig.prefix) &&
			bytes.Equal(header[sig.offset:sig.offset+len(sig.prefix)], sig.prefix) {
			return true
		}
	}
	return false
}

// detectMimeType 用文件头推测 Content-Type（写入对象存储，供浏览器预览）。
func detectMimeType(header []byte, ext string) string {
	if len(header) == 0 {
		return "application/octet-stream"
	}
	ct := http.DetectContentType(header)
	if ct == "application/octet-stream" {
		if byExt := mime.TypeByExtension(ext); byExt != "" {
			return byExt
		}
	}
	return ct
}

// sanitizeName 只保留文件名部分并限制长度（Name 列 varchar(256)，按字符截断）。
func sanitizeName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || name == "/" {
		return "未命名文件"
	}
	if runes := []rune(name); len(runes) > maxNameRunes {
		name = string(runes[:maxNameRunes])
	}
	return name
}

// humanSize 把字节数格式化成便于阅读的形式。
func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%dMB", n>>20)
	case n >= 1<<10:
		return fmt.Sprintf("%dKB", n>>10)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func sortedExts(set map[string]struct{}) []string {
	exts := make([]string, 0, len(set))
	for ext := range set {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return exts
}
