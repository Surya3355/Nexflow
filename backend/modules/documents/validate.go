// Package documents holds case documents: upload, latest-version replacement,
// private download links and the stage document requirement (spec §4.4).
package documents

import (
	"errors"
	"net/http"
)

// Upload limits and allowed types (spec §4.4, round 5).
const MaxFileSize = 10 * 1024 * 1024 // 10485760 bytes

var (
	ErrEmpty           = errors.New("documents: the file is empty")
	ErrTooLarge        = errors.New("documents: the file is larger than 10 MB")
	ErrUnsupportedType = errors.New("documents: only PDF, JPG and PNG files are accepted")
)

// allowed maps the content type sniffed from the file's first bytes to the
// type stored. The file name and extension are not trusted (review focus 3).
var allowed = map[string]bool{
	"application/pdf": true,
	"image/jpeg":      true,
	"image/png":       true,
}

// ValidateFile checks the file's content, not its name: type from the first
// bytes, size between 1 byte and MaxFileSize. It returns the content type to store.
func ValidateFile(name string, content []byte) (string, error) {
	_ = name // name is stored, never trusted for the type
	if len(content) == 0 {
		return "", ErrEmpty
	}
	if len(content) > MaxFileSize {
		return "", ErrTooLarge
	}
	sniffed := http.DetectContentType(content)
	if !allowed[sniffed] {
		return "", ErrUnsupportedType
	}
	return sniffed, nil
}
