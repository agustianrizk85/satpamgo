package media

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrInvalidFileType = errors.New("invalid file type")
	ErrInvalidDate     = errors.New("invalid date")
)

type SaveInput struct {
	Category string
	PlaceID  string
	UserID   string
	Date     string
	NameHint string
	File     multipart.File
}

type SaveResult struct {
	ObjectKey string `json:"objectKey"`
	PhotoURL  string `json:"photoUrl"`
	MimeType  string `json:"mimeType"`
	Size      int64  `json:"size"`
}

type Service struct {
	root string
}

const defaultSystemCheckoutAssetPath = "system/attendance/check-out-by-system.svg"

func NewService(root string) *Service {
	return &Service{root: root}
}

func (s *Service) Root() string {
	return s.root
}

func (s *Service) EnsureDefaultSystemCheckoutAsset() (string, error) {
	fullPath := filepath.Join(s.root, filepath.FromSlash(defaultSystemCheckoutAssetPath))
	if _, err := os.Stat(fullPath); err == nil {
		return "/uploads/" + defaultSystemCheckoutAssetPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", err
	}

	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
<rect width="1200" height="630" fill="#0f172a"/>
<rect x="40" y="40" width="1120" height="550" rx="32" fill="#111827" stroke="#38bdf8" stroke-width="4"/>
<text x="600" y="250" text-anchor="middle" font-family="Arial, sans-serif" font-size="56" font-weight="700" fill="#f8fafc">CHECK OUT BY SYSTEM</text>
<text x="600" y="330" text-anchor="middle" font-family="Arial, sans-serif" font-size="28" fill="#cbd5e1">Auto checkout applied after grace period</text>
<text x="600" y="390" text-anchor="middle" font-family="Arial, sans-serif" font-size="22" fill="#7dd3fc">satpam-go</text>
</svg>
`

	if err := os.WriteFile(fullPath, []byte(svg), 0o644); err != nil {
		return "", err
	}

	return "/uploads/" + defaultSystemCheckoutAssetPath, nil
}

func (s *Service) Save(input SaveInput) (SaveResult, error) {
	dateValue, err := time.Parse("2006-01-02", strings.TrimSpace(input.Date))
	if err != nil {
		return SaveResult{}, ErrInvalidDate
	}

	contentType, ext, err := sniffImage(input.File)
	if err != nil {
		return SaveResult{}, err
	}

	safeName := sanitizeName(input.NameHint)
	if safeName == "" {
		safeName = input.Category
	}
	fileName := fmt.Sprintf("%s-%s%s", safeName, randomSuffix(6), ext)

	objectKey := path.Join(
		"places",
		input.PlaceID,
		"users",
		input.UserID,
		dateValue.Format("2006-01-02"),
		input.Category,
		fileName,
	)
	fullPath := filepath.Join(s.root, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return SaveResult{}, err
	}

	dst, err := os.Create(fullPath)
	if err != nil {
		return SaveResult{}, err
	}
	defer dst.Close()

	size, err := io.Copy(dst, input.File)
	if err != nil {
		return SaveResult{}, err
	}

	return SaveResult{
		ObjectKey: objectKey,
		PhotoURL:  "/uploads/" + objectKey,
		MimeType:  contentType,
		Size:      size,
	}, nil
}

func (s *Service) CleanupAttendanceExpired(now time.Time) error {
	cutoff := firstDayOfPreviousMonth(now)
	return filepath.WalkDir(s.root, func(currentPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() || !strings.EqualFold(entry.Name(), "attendance") {
			return nil
		}

		dateDir := filepath.Base(filepath.Dir(currentPath))
		dateValue, parseErr := time.Parse("2006-01-02", dateDir)
		if parseErr != nil {
			return nil
		}
		if dateValue.Before(cutoff) {
			if removeErr := os.RemoveAll(currentPath); removeErr != nil {
				return removeErr
			}
			return filepath.SkipDir
		}
		return nil
	})
}

func sniffImage(file multipart.File) (contentType string, ext string, err error) {
	header := make([]byte, 512)
	n, readErr := file.Read(header)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", "", readErr
	}
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		return "", "", seekErr
	}

	contentType = http.DetectContentType(header[:n])
	switch contentType {
	case "image/jpeg":
		return contentType, ".jpg", nil
	case "image/webp":
		return contentType, ".webp", nil
	default:
		return "", "", ErrInvalidFileType
	}
}

func sanitizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	return strings.Trim(builder.String(), "-_")
}

func randomSuffix(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func firstDayOfPreviousMonth(now time.Time) time.Time {
	firstDayOfCurrentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return firstDayOfCurrentMonth.AddDate(0, -1, 0)
}
