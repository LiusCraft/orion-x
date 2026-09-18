package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/assets"
	"github.com/liuscraft/orion-x/internal/logging"
)

// maxMultipartOverhead 是 multipart 边界与其它表单字段的余量。
const maxMultipartOverhead = 1 << 20

// AssetHandler 是资源（上传文件）的 HTTP 入口。
type AssetHandler struct {
	svc *assets.Service
}

// NewAssetHandler 构造 AssetHandler；svc 为 nil 表示未配置对象存储。
func NewAssetHandler(svc *assets.Service) *AssetHandler {
	return &AssetHandler{svc: svc}
}

func (h *AssetHandler) needSvc(c *gin.Context) bool {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "未配置对象存储，请先在 manager.yaml 中配置 storage 段"})
		return false
	}
	return true
}

// Upload POST /api/assets — multipart：file + purpose
func (h *AssetHandler) Upload(c *gin.Context) {
	if !h.needSvc(c) {
		return
	}
	// 全局体积上限兜底：超限直接 413，不会把整个 body 读进内存。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.svc.MaxUploadSize()+maxMultipartOverhead)

	file, err := c.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "文件超过单文件上限"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传文件"})
		return
	}
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取上传文件失败"})
		return
	}
	defer func() { _ = f.Close() }()

	asset, err := h.svc.Create(c.Request.Context(), assets.Upload{
		OwnerID:  middleware.UserID(c),
		Purpose:  assets.Purpose(c.PostForm("purpose")),
		FileName: file.Filename,
		Size:     file.Size,
		Body:     f,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	view, err := h.svc.View(c.Request.Context(), asset)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, view)
}

// List GET /api/assets?purpose=&q=&page=&page_size=
func (h *AssetHandler) List(c *gin.Context) {
	if !h.needSvc(c) {
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	q := assets.ListQuery{
		OwnerID:  middleware.UserID(c),
		Purpose:  assets.Purpose(c.Query("purpose")),
		Name:     c.Query("q"),
		Page:     page,
		PageSize: pageSize,
	}.Normalize()

	list, total, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		h.writeError(c, err)
		return
	}
	views, err := h.svc.Views(c.Request.Context(), list)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"assets":    views,
		"total":     total,
		"page":      q.Page,
		"page_size": q.PageSize,
	})
}

// Get GET /api/assets/:id
func (h *AssetHandler) Get(c *gin.Context) {
	if !h.needSvc(c) {
		return
	}
	asset, err := h.svc.Get(c.Request.Context(), middleware.UserID(c), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	view, err := h.svc.View(c.Request.Context(), asset)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// URL GET /api/assets/:id/url?download=1
func (h *AssetHandler) URL(c *gin.Context) {
	if !h.needSvc(c) {
		return
	}
	asset, err := h.svc.Get(c.Request.Context(), middleware.UserID(c), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	url, ttl, err := h.svc.PresignURL(c.Request.Context(), asset, c.Query("download") == "1")
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url, "expires_in": int(ttl.Seconds())})
}

// Delete DELETE /api/assets/:id
func (h *AssetHandler) Delete(c *gin.Context) {
	if !h.needSvc(c) {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), middleware.UserID(c), c.Param("id")); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// writeError 把资源服务的错误映射成 HTTP 响应。
func (h *AssetHandler) writeError(c *gin.Context, err error) {
	var invalid *assets.ValidationError
	switch {
	case errors.As(err, &invalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": invalid.Reason})
	case errors.Is(err, assets.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "无权访问该资源；音色样本与知识库文档请在对应页面管理"})
	case errors.Is(err, assets.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
	case errors.Is(err, assets.ErrStorage):
		logging.Errorf("Asset storage error: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "对象存储操作失败，请重试"})
	default:
		logging.Errorf("Asset internal error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}
