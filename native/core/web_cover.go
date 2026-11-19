package core

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// OpenCover returns the cached browser-ready image for station metadata.
func (library *WebLibrary) OpenCover(ctx context.Context, id, address string) (*os.File, string, error) {
	path, err := library.covers.load(ctx, LibraryDrama{ID: id, Source: SourceFromDramaID(id), Cover: address}, false)
	if err != nil {
		return nil, "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	var header [512]byte
	n, err := file.ReadAt(header[:], 0)
	if err != nil && err != io.EOF {
		file.Close()
		return nil, "", err
	}
	mediaType := http.DetectContentType(header[:n])
	if n >= 12 && string(header[4:8]) == "ftyp" {
		switch string(header[8:12]) {
		case "avif", "avis":
			mediaType = "image/avif"
		}
	}
	return file, mediaType, nil
}

func webCoverJPEG(ctx context.Context, data []byte) ([]byte, error) {
	if !isHEICImage(data) {
		return data, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "guoapp-cover-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	input, output := filepath.Join(dir, "cover.heic"), filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(input, data, 0600); err != nil {
		return nil, err
	}
	// Limit decoder parallelism. libheif's default image security limits remain enabled.
	if err := exec.CommandContext(ctx, "heif-convert", "--quiet", "--codec-threads", "1", "--tile-threads", "1", "-q", "85", input, output).Run(); err != nil {
		return nil, err
	}
	file, err := os.Open(output)
	if os.IsNotExist(err) {
		file, err = os.Open(filepath.Join(dir, "cover-1.jpg"))
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	jpeg, err := io.ReadAll(io.LimitReader(file, nativeCoverMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(jpeg) > nativeCoverMaxBytes || http.DetectContentType(jpeg) != "image/jpeg" {
		return nil, errors.New("封面转码未返回有效 JPEG")
	}
	return jpeg, nil
}
