package converter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Converter struct {
	ffmpegPath string
}

func NewConverter() (*Converter, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: please install ffmpeg")
	}
	return &Converter{ffmpegPath: path}, nil
}

func (c *Converter) ConvertHlsToWebp(ctx context.Context, hlsURL, targetPath string, scale, fps, quality int) error {
	dir := filepath.Dir(targetPath)
	tempFile := filepath.Join(dir, fmt.Sprintf(".animart.%d.tmp.webp", time.Now().UnixNano()))
	defer os.Remove(tempFile)

	// Filter chain: scale down while maintaining aspect ratio, and enforce target FPS
	filterChain := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,fps=%d", scale, scale, fps)

	args := []string{
		"-y",
		"-loglevel", "error",
		"-i", hlsURL,
		"-vf", filterChain,
		"-c:v", "libwebp",
		"-preset", "photo",
		"-compression_level", "4",
		"-q:v", fmt.Sprintf("%d", quality),
		"-loop", "0",
		"-an",
		tempFile,
	}

	cmd := exec.CommandContext(ctx, c.ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
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
