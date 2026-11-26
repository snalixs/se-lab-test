package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxImageBytes = 8 << 20
const maxPostUploadBytes = 10*maxImageBytes + 1<<20

var mediaNamePattern = regexp.MustCompile(`^[a-f0-9]{32}\.(jpg|png|webp)$`)

func prepareMediaDir() (string, error) {
	dir := os.Getenv("MEDIA_DIR")
	if dir == "" {
		dir = "media"
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return "", err
	}
	return absolute, nil
}

func (s *server) serveMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !mediaNamePattern.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, filepath.Join(s.mediaDir, name))
}

func removeMedia(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func (s *server) readMultipartPost(w http.ResponseWriter, r *http.Request) (postInput, []string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPostUploadBytes)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		return postInput{}, nil, bad("post is too large or has invalid form data")
	}
	defer r.MultipartForm.RemoveAll()

	body := postInput{
		Content:    r.FormValue("content"),
		Visibility: r.FormValue("visibility"),
		Hashtags:   r.MultipartForm.Value["hashtags"],
	}
	if body.Visibility == "" {
		body.Visibility = "public"
	}
	files := r.MultipartForm.File["images"]
	if len(files) > 10 {
		return postInput{}, nil, bad("at most 10 images are allowed")
	}
	if err := validatePost(&body, 1); err != nil {
		return postInput{}, nil, err
	}

	saved := make([]string, 0, len(files))
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			removeMedia(saved)
			return postInput{}, nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			removeMedia(saved)
			return postInput{}, nil, fmt.Errorf("read image: %w", errors.Join(readErr, closeErr))
		}
		if len(data) == 0 || len(data) > maxImageBytes {
			removeMedia(saved)
			return postInput{}, nil, bad("each image must be 8 MB or less")
		}

		var extension string
		switch http.DetectContentType(data[:min(len(data), 512)]) {
		case "image/jpeg":
			extension = ".jpg"
		case "image/png":
			extension = ".png"
		case "image/webp":
			extension = ".webp"
		default:
			removeMedia(saved)
			return postInput{}, nil, bad("images must be JPEG, PNG or WebP")
		}

		random := make([]byte, 16)
		if _, err := rand.Read(random); err != nil {
			removeMedia(saved)
			return postInput{}, nil, err
		}
		name := hex.EncodeToString(random) + extension
		path := filepath.Join(s.mediaDir, name)
		output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			removeMedia(saved)
			return postInput{}, nil, err
		}
		_, writeErr := output.Write(data)
		closeErr = output.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			removeMedia(saved)
			return postInput{}, nil, fmt.Errorf("save image: %w", errors.Join(writeErr, closeErr))
		}
		saved = append(saved, path)
		body.Images = append(body.Images, "/media/"+name)
	}
	return body, saved, nil
}

func isLocalMediaURL(value string) bool {
	return strings.HasPrefix(value, "/media/") &&
		mediaNamePattern.MatchString(strings.TrimPrefix(value, "/media/"))
}
