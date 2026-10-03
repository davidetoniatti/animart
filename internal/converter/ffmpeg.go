package converter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const conversionTimeout = 10 * time.Minute

type Converter struct {
	ffmpegPath string
	httpClient *http.Client
}

func NewConverter() (*Converter, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: please install ffmpeg")
	}
	return &Converter{
		ffmpegPath: path,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *Converter) ConvertHlsToWebp(ctx context.Context, hlsURL, targetPath string, scale, fps, quality int) error {
	source, err := c.resolveVariant(ctx, hlsURL, scale)
	if err != nil {
		return fmt.Errorf("failed to read HLS playlist: %w", err)
	}

	dir := filepath.Dir(targetPath)
	tempFile := filepath.Join(dir, fmt.Sprintf(".animart.%d.tmp.webp", time.Now().UnixNano()))
	defer os.Remove(tempFile)

	// Center-crop to a square (tall videos lose top and bottom), then resize
	// and resample.
	filterChain := fmt.Sprintf("crop=min(iw\\,ih):min(iw\\,ih),scale=%d:%d,fps=%d", scale, scale, fps)

	args := []string{
		"-nostdin",
		"-y",
		"-loglevel", "error",
		"-i", source,
		"-vf", filterChain,
		"-c:v", "libwebp",
		"-preset", "photo",
		"-compression_level", "4",
		"-q:v", fmt.Sprintf("%d", quality),
		"-loop", "0",
		"-an",
		tempFile,
	}

	ctx, cancel := context.WithTimeout(ctx, conversionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("ffmpeg timed out after %v", conversionTimeout)
		}
		return fmt.Errorf("ffmpeg conversion failed: %w (stderr: %s)", err, stderr.String())
	}

	info, err := os.Stat(tempFile)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("ffmpeg generated an empty or non-existent file")
	}

	// Atomic rename to final cover.webp
	if err := os.Rename(tempFile, targetPath); err != nil {
		return fmt.Errorf("failed to move temp file to %s: %w", targetPath, err)
	}

	return nil
}
