package appversions

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var ErrInvalidAPK = errors.New("invalid apk file")

type Storage struct {
	root string
}

type UploadResult struct {
	ObjectKey   string `json:"objectKey"`
	DownloadURL string `json:"downloadUrl"`
	FileName    string `json:"fileName"`
	Size        int64  `json:"size"`
}

func NewStorage(root string) *Storage {
	return &Storage{root: root}
}

func (s *Storage) SaveAPK(placeID, userID, versionName, originalName string, file multipart.File) (UploadResult, error) {
	fileName, err := normalizeAPKName(versionName, originalName, file)
	if err != nil {
		return UploadResult{}, err
	}

	objectKey := path.Join("app-versions", placeID, userID, fileName)
	fullPath := filepath.Join(s.root, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return UploadResult{}, err
	}

	dst, err := os.Create(fullPath)
	if err != nil {
		return UploadResult{}, err
	}
	defer dst.Close()

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return UploadResult{}, err
	}
	size, err := io.Copy(dst, file)
	if err != nil {
		return UploadResult{}, err
	}

	return UploadResult{
		ObjectKey:   objectKey,
		DownloadURL: "/uploads/" + objectKey,
		FileName:    fileName,
		Size:        size,
	}, nil
}

func (s *Storage) SaveMasterAPK(versionName, originalName string, file multipart.File) (UploadResult, error) {
	fileName, err := normalizeAPKName(versionName, originalName, file)
	if err != nil {
		return UploadResult{}, err
	}

	objectKey := path.Join("app-version-masters", fileName)
	fullPath := filepath.Join(s.root, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return UploadResult{}, err
	}

	dst, err := os.Create(fullPath)
	if err != nil {
		return UploadResult{}, err
	}
	defer dst.Close()

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return UploadResult{}, err
	}
	size, err := io.Copy(dst, file)
	if err != nil {
		return UploadResult{}, err
	}

	return UploadResult{
		ObjectKey:   objectKey,
		DownloadURL: "/uploads/" + objectKey,
		FileName:    fileName,
		Size:        size,
	}, nil
}

func normalizeAPKName(versionName, originalName string, file multipart.File) (string, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(originalName)))
	if ext != ".apk" {
		return "", ErrInvalidAPK
	}

	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	contentType := http.DetectContentType(header[:n])
	switch contentType {
	case "application/vnd.android.package-archive", "application/octet-stream", "application/zip":
	default:
		if n > 0 {
			return "", ErrInvalidAPK
		}
	}

	safeVersion := sanitizePart(versionName)
	if safeVersion == "" {
		safeVersion = "app"
	}
	return fmt.Sprintf("%s-%s.apk", safeVersion, randomSuffix(4)), nil
}

func sanitizePart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")

	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-_.")
}

func randomSuffix(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(buf)
}
