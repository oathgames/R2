package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReferenceImage holds a loaded image ready to send to Gemini.
type ReferenceImage struct {
	Path     string
	MimeType string
	Base64   string
}

// loadReferenceImages scans a directory for image files and loads them as base64.
// Supports .png, .jpg, .jpeg, .webp. Skips non-image files silently.
// Returns nil (not error) if the directory doesn't exist or is empty.
func loadReferenceImages(dir string) ([]ReferenceImage, error) {
	if dir == "" {
		return nil, nil
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil // No references dir — that's fine
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read references dir: %w", err)
	}

	var refs []ReferenceImage
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		mime := ""
		switch ext {
		case ".png":
			mime = "image/png"
		case ".jpg", ".jpeg":
			mime = "image/jpeg"
		case ".webp":
			mime = "image/webp"
		default:
			continue // Skip non-image files
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("  [WARN] Cannot read %s: %v\n", entry.Name(), err)
			continue
		}

		// Limit: Gemini has a ~20MB request limit. Skip huge images.
		if len(data) > 10*1024*1024 {
			fmt.Printf("  [WARN] Skipping %s (>10MB)\n", entry.Name())
			continue
		}

		refs = append(refs, ReferenceImage{
			Path:     path,
			MimeType: mime,
			Base64:   base64.StdEncoding.EncodeToString(data),
		})
		fmt.Printf("  Reference: %s (%.0f KB)\n", entry.Name(), float64(len(data))/1024)
	}

	return refs, nil
}

// loadSpecificImages loads specific image files by path.
func loadSpecificImages(paths []string) ([]ReferenceImage, error) {
	var refs []ReferenceImage
	for _, path := range paths {
		ext := strings.ToLower(filepath.Ext(path))
		mime := ""
		switch ext {
		case ".png":
			mime = "image/png"
		case ".jpg", ".jpeg":
			mime = "image/jpeg"
		case ".webp":
			mime = "image/webp"
		default:
			return nil, fmt.Errorf("unsupported image format: %s", ext)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %w", path, err)
		}

		refs = append(refs, ReferenceImage{
			Path:     path,
			MimeType: mime,
			Base64:   base64.StdEncoding.EncodeToString(data),
		})
	}
	return refs, nil
}
