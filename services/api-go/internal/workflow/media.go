package workflow

import (
	"bytes"
	"mime"
	"net/http"
	"path"
	"strings"

	"lokasolve/services/api-go/internal/platform/httpx"
)

const maxMediaBytes = 4_000_000

const publicSummary = "Exact address, coordinates, and photos are not included."

// InspectUpload identifies a file from its bytes. The browser Content-Type is
// only used to reject a declaration that disagrees with the bytes.
func InspectUpload(fileName, declared string, body []byte) (contentType, storedName string, err error) {
	if len(body) == 0 {
		return "", "", httpx.New(http.StatusBadRequest, "validation_error", "The file is empty.")
	}
	if len(body) > maxMediaBytes {
		return "", "", httpx.New(http.StatusBadRequest, "validation_error", "The file must be under 4 MB.")
	}
	if denied(body) {
		return "", "", httpx.New(http.StatusBadRequest, "validation_error", "That file type is not supported.")
	}
	kind, ok := detect(body)
	if !ok {
		return "", "", httpx.New(http.StatusBadRequest, "validation_error", "The file is not a complete JPEG, PNG, WEBP, GIF, MP3, M4A, MP4, or WEBM.")
	}
	ext := strings.ToLower(path.Ext(path.Base(strings.TrimSpace(fileName))))
	contentType, ok = resolveType(kind, ext)
	if !ok {
		return "", "", httpx.New(http.StatusBadRequest, "validation_error", "The file name does not match the file contents.")
	}
	if strings.TrimSpace(declared) != "" {
		mediaType, _, parseErr := mime.ParseMediaType(declared)
		if parseErr != nil || !strings.EqualFold(mediaType, contentType) {
			return "", "", httpx.New(http.StatusBadRequest, "validation_error", "The file content does not match the declared type.")
		}
	}
	return contentType, "media" + canonicalExt(contentType), nil
}

func denied(body []byte) bool {
	if len(body) >= 2 && body[0] == 'M' && body[1] == 'Z' {
		return true
	}
	if len(body) >= 4 && body[0] == 0x7f && body[1] == 'E' && body[2] == 'L' && body[3] == 'F' {
		return true
	}
	if len(body) >= 4 && body[0] == 'P' && body[1] == 'K' && body[2] == 0x03 && body[3] == 0x04 {
		return true
	}
	if len(body) >= 2 && body[0] == '#' && body[1] == '!' {
		return true
	}
	return false
}

func detect(body []byte) (string, bool) {
	if len(body) >= 4 && body[0] == 0xFF && body[1] == 0xD8 && body[2] == 0xFF && bytes.Contains(body, []byte{0xFF, 0xD9}) {
		return "jpeg", true
	}
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if len(body) >= len(png) && bytes.Equal(body[:len(png)], png) && bytes.Contains(body, []byte("IEND")) {
		return "png", true
	}
	if len(body) >= 6 && (bytes.HasPrefix(body, []byte("GIF87a")) || bytes.HasPrefix(body, []byte("GIF89a"))) && body[len(body)-1] == 0x3B {
		return "gif", true
	}
	if len(body) >= 12 && bytes.Equal(body[:4], []byte("RIFF")) && bytes.Equal(body[8:12], []byte("WEBP")) {
		return "webp", true
	}
	if len(body) >= 10 && bytes.HasPrefix(body, []byte("ID3")) {
		return "mp3", true
	}
	if len(body) >= 2 && body[0] == 0xFF && body[1]&0xE0 == 0xE0 && len(body) >= 16 {
		return "mp3", true
	}
	if len(body) >= 12 && bytes.Equal(body[4:8], []byte("ftyp")) {
		return "mp4:" + string(body[8:12]), true
	}
	if len(body) >= 4 && body[0] == 0x1A && body[1] == 0x45 && body[2] == 0xDF && body[3] == 0xA3 && bytes.Contains(body[:min(len(body), 256)], []byte("webm")) {
		return "webm", true
	}
	return "", false
}

func resolveType(kind, ext string) (string, bool) {
	switch kind {
	case "jpeg":
		if ext == ".jpg" || ext == ".jpeg" {
			return "image/jpeg", true
		}
	case "png":
		if ext == ".png" {
			return "image/png", true
		}
	case "gif":
		if ext == ".gif" {
			return "image/gif", true
		}
	case "webp":
		if ext == ".webp" {
			return "image/webp", true
		}
	case "mp3":
		if ext == ".mp3" {
			return "audio/mpeg", true
		}
	case "webm":
		if ext == ".webm" {
			return "video/webm", true
		}
		if ext == ".weba" {
			return "audio/webm", true
		}
	default:
		if strings.HasPrefix(kind, "mp4:") {
			brand := kind[4:]
			switch {
			case (brand == "M4A " || brand == "M4B ") && ext == ".m4a":
				return "audio/mp4", true
			case (brand == "isom" || brand == "iso2" || brand == "mp41" || brand == "mp42" || brand == "avc1") && ext == ".mp4":
				return "video/mp4", true
			}
		}
	}
	return "", false
}

func canonicalExt(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4":
		return ".m4a"
	case "audio/webm":
		return ".weba"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	default:
		return ".bin"
	}
}
