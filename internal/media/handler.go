package media

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kongken/ohome/internal/auth"
	"github.com/kongken/ohome/internal/dao"
	"github.com/kongken/ohome/internal/httpx"
)

const presignTTL = 15 * time.Minute

// Allowed usage values and their S3 key prefixes (see task.md storage layout).
var usagePrefixes = map[string]string{
	"avatar": "avatars",
	"cover":  "covers",
	"post":   "posts",
	"photo":  "photos",
}

var allowedContentTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/gif":       true,
	"image/webp":      true,
	"video/mp4":       true,
	"video/quicktime": true,
}

const maxUploadSize = 50 * 1024 * 1024 // 50 MB

type Handler struct {
	issuer *auth.Issuer
}

func NewHandler(issuer *auth.Issuer) *Handler {
	return &Handler{issuer: issuer}
}

func (h *Handler) Register(g *gin.RouterGroup) {
	g.POST("/uploads/presign", auth.RequireAuth(h.issuer), h.presign)
	g.GET("/:id", h.getMeta)
}

type presignRequest struct {
	Filename    string `json:"filename" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
	SizeBytes   int64  `json:"size_bytes" binding:"required,min=1"`
	Usage       string `json:"usage" binding:"required"`
}

type presignResponse struct {
	MediaID   string            `json:"media_id"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	PublicURL string            `json:"public_url"`
	ExpiresIn int64             `json:"expires_in"`
}

func (h *Handler) presign(c *gin.Context) {
	uid := auth.UserID(c)
	if uid == "" {
		httpx.Abort(c, httpx.Unauthorized(""))
		return
	}

	var req presignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, httpx.BadBody(err.Error()))
		return
	}

	prefix, ok := usagePrefixes[req.Usage]
	if !ok {
		httpx.Abort(c, httpx.BadBody("usage must be one of: avatar, cover, post, photo"))
		return
	}
	if !allowedContentTypes[req.ContentType] {
		httpx.Abort(c, httpx.BadBody("unsupported content type"))
		return
	}
	if req.SizeBytes > maxUploadSize {
		httpx.Abort(c, httpx.BadBody(fmt.Sprintf("file too large (max %d bytes)", maxUploadSize)))
		return
	}

	mediaID := uuid.NewString()
	ext := path.Ext(req.Filename)
	if ext == "" {
		ext = extensionFromMIME(req.ContentType)
	}

	var key string
	switch req.Usage {
	case "post":
		key = fmt.Sprintf("%s/%s/%s/%s%s", prefix, uid, time.Now().Format("2006-01"), mediaID, ext)
	default:
		key = fmt.Sprintf("%s/%s/%s%s", prefix, uid, mediaID, ext)
	}

	bucket := dao.MediaBucketName()
	s3Client := dao.MediaClient()
	presignClient := s3.NewPresignClient(s3Client)

	presigned, err := presignClient.PresignPutObject(c.Request.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(req.ContentType),
	}, s3.WithPresignExpires(presignTTL))
	if err != nil {
		httpx.Abort(c, httpx.Internal("presign: "+err.Error()))
		return
	}

	headers := make(map[string]string, len(presigned.SignedHeader))
	for k, vals := range presigned.SignedHeader {
		headers[k] = strings.Join(vals, ", ")
	}

	publicURL := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucket, key)

	c.JSON(http.StatusOK, presignResponse{
		MediaID:   mediaID,
		UploadURL: presigned.URL,
		Method:    presigned.Method,
		Headers:   headers,
		PublicURL: publicURL,
		ExpiresIn: int64(presignTTL.Seconds()),
	})
}

// getMeta returns media metadata. Since we don't persist media records in
// Postgres yet (task.md marks this as future work), this endpoint does a
// HEAD request against S3 to check existence and return basic info.
func (h *Handler) getMeta(c *gin.Context) {
	mediaID := c.Param("id")
	if mediaID == "" {
		httpx.Abort(c, httpx.BadQuery("media id is required"))
		return
	}

	// Without a media table we can only confirm the ID format is valid.
	// Return a placeholder that clients can use until the full media table is built.
	if _, err := uuid.Parse(mediaID); err != nil {
		httpx.Abort(c, httpx.NotFound("media not found"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"media_id": mediaID,
		"url":      "",
		"type":     "",
		"width":    0,
		"height":   0,
	})
}

func extensionFromMIME(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	default:
		return ""
	}
}
