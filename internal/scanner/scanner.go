package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

var audioExtensions = map[string]bool{
	".flac": true,
	".mp3":  true,
	".m4a":  true,
	".ogg":  true,
	".opus": true,
	".aac":  true,
	".wav":  true,
	".wma":  true,
}

type Album struct {
	Path   string
	Artist string
	Album  string
}

func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return audioExtensions[ext]
}

func isExcluded(dirName string, excludes []string) bool {
	for _, excl := range excludes {
		if strings.EqualFold(excl, dirName) {
			return true
		}
	}
	return false
}

// Scan returns the album folders under root: folders with an audio file
// whose tags carry artist and album. Folders named in excludes are skipped at
// any depth. Folders that already contain coverName are left out unless force
// is set.
func Scan(root string, excludes []string, coverName string, force bool) ([]Album, error) {
	var albums []Album
	done := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped
		}
		if d.IsDir() {
			if path != root && isExcluded(d.Name(), excludes) {
				return filepath.SkipDir
			}
			return nil
		}

		dir := filepath.Dir(path)
		if done[dir] || !isAudioFile(d.Name()) {
			return nil
		}
		if !force {
			if _, err := os.Stat(filepath.Join(dir, coverName)); err == nil {
				done[dir] = true
				return nil
			}
		}

		// A file without usable tags leaves the folder open to the next one.
		if artist, album, err := readAudioTags(path); err == nil {
			albums = append(albums, Album{Path: dir, Artist: artist, Album: album})
			done[dir] = true
		}
		return nil
	})

	return albums, err
}

func readAudioTags(filePath string) (artist, album string, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return "", "", err
	}

	album = strings.TrimSpace(m.Album())
	artist = strings.TrimSpace(m.AlbumArtist())
	if artist == "" {
		artist = strings.TrimSpace(m.Artist())
	}

	if artist == "" || album == "" {
		return "", "", fmt.Errorf("missing artist or album tags")
	}

	return artist, album, nil
}
