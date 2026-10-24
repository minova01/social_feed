package interfaceshttpupload

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	inframetrics "social/internal/infra/metrics"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxImageBytes = 20 << 20
const multipartOverheadBytes = 1 << 20
const sniffBytes = 512

var allowedImageExt = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".webp": {},
}

var allowedImageMIME = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

var errInvalidUploadExtension = errors.New("unsupported upload extension")
var errInvalidUploadMIME = errors.New("unsupported upload content type")
var errUploadTooLarge = errors.New("upload file is too large")

type Handler struct {
	root string
}

type uploadResponse struct {
	URL      string `json:"url"`
	Kind     string `json:"kind"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

func New(root string) *Handler {
	return &Handler{root: root}
}

func (h *Handler) Create(c *gin.Context) {
	start := time.Now()
	kind := "unknown"
	var resultErr error
	defer func() {
		inframetrics.ObserveUpload(kind, time.Since(start), resultErr)
	}()

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImageBytes+multipartOverheadBytes)
	file, err := c.FormFile("file")
	if err != nil {
		resultErr = err
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	normalizedKind, ok := normalizeKind(c.PostForm("kind"))
	if !ok {
		resultErr = errors.New("invalid upload kind")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload kind"})
		return
	}
	kind = normalizedKind

	ext, err := validateImage(file)
	if err != nil {
		resultErr = err
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filename := fmt.Sprintf("%d-%s%s", time.Now().UnixMilli(), randomSuffix(), ext)
	directory := kind + "s"
	targetDir := filepath.Join(h.root, directory)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		resultErr = err
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare upload directory"})
		return
	}
	if err := c.SaveUploadedFile(file, filepath.Join(targetDir, filename)); err != nil {
		resultErr = err
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save upload"})
		return
	}

	c.JSON(http.StatusCreated, uploadResponse{
		URL:      "/uploads/" + directory + "/" + filename,
		Kind:     kind,
		Filename: filename,
		Size:     file.Size,
	})
}

func normalizeKind(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "post":
		return "post", true
	case "avatar":
		return "avatar", true
	default:
		return "", false
	}
}

func validateImage(file *multipart.FileHeader) (string, error) {
	if file.Size <= 0 {
		return "", errInvalidUploadMIME
	}
	if file.Size > maxImageBytes {
		return "", errUploadTooLarge
	}
	ext := strings.ToLower(filepath.Ext(filepath.Base(file.Filename)))
	if _, ok := allowedImageExt[ext]; !ok {
		return "", errInvalidUploadExtension
	}
	mimeType, err := sniffUploadMIME(file)
	if err != nil {
		return "", err
	}
	if _, ok := allowedImageMIME[mimeType]; !ok {
		return "", errInvalidUploadMIME
	}
	return ext, nil
}

func sniffUploadMIME(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", errInvalidUploadMIME
	}
	defer file.Close()

	header := make([]byte, sniffBytes)
	n, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", errInvalidUploadMIME
	}
	if n == 0 {
		return "", errInvalidUploadMIME
	}
	return http.DetectContentType(header[:n]), nil
}

func randomSuffix() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
