package main

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestMultipartPostImageLimit(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{10, 11} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			var payload bytes.Buffer
			writer := multipart.NewWriter(&payload)
			if err := writer.WriteField("content", "Morning run"); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < count; i++ {
				file, err := writer.CreateFormFile("images", "photo.png")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.Write(imageData.Bytes()); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("POST", "/api/posts", &payload)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			directory := t.TempDir()
			s := &server{mediaDir: directory}
			body, saved, err := s.readMultipartPost(httptest.NewRecorder(), request)
			if count == 11 {
				if err == nil || !strings.Contains(err.Error(), "10 images") {
					t.Fatalf("expected image limit error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(body.Images) != 10 || len(saved) != 10 {
				t.Fatalf("got %d URLs and %d files", len(body.Images), len(saved))
			}
			if !validMediaURL(body.Images[0]) {
				t.Fatalf("uploaded URL was rejected: %s", body.Images[0])
			}
			for _, path := range saved {
				if _, err := os.Stat(path); err != nil {
					t.Fatal(err)
				}
			}
			imageRequest := httptest.NewRequest("GET", body.Images[0], nil)
			imageRequest.SetPathValue("file", strings.TrimPrefix(body.Images[0], "/media/"))
			imageResponse := httptest.NewRecorder()
			s.serveMedia(imageResponse, imageRequest)
			if imageResponse.Code != 200 || !bytes.Equal(imageResponse.Body.Bytes(), imageData.Bytes()) {
				t.Fatalf("uploaded image was not served intact: status %d", imageResponse.Code)
			}
			removeMedia(saved)
		})
	}
}
