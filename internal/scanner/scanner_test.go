package scanner

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// writeMP3 writes a file holding only an ID3v2.3 tag with the given frames.
func writeMP3(t *testing.T, path string, frames map[string]string) {
	t.Helper()
	var body []byte
	for id, text := range frames {
		data := append([]byte{0}, text...) // ISO-8859-1 text
		body = append(body, id...)
		body = binary.BigEndian.AppendUint32(body, uint32(len(data)))
		body = append(body, 0, 0)
		body = append(body, data...)
	}
	n := len(body)
	header := []byte{'I', 'D', '3', 3, 0, 0, byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
	writeFile(t, path, string(append(header, body...)))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	tags := func(artist, album string) map[string]string {
		return map[string]string{"TPE1": artist, "TALB": album}
	}

	writeMP3(t, filepath.Join(root, "Korn", "Issues", "01.mp3"), tags("Korn", "Issues"))
	writeMP3(t, filepath.Join(root, "Korn", "Requiem", "01.mp3"), tags("Korn", "Requiem"))
	writeFile(t, filepath.Join(root, "Korn", "Requiem", "cover.webp"), "webp")
	writeMP3(t, filepath.Join(root, "Mixed", "Album", "01.mp3"), map[string]string{"TPE1": "Nobody"})
	writeMP3(t, filepath.Join(root, "Mixed", "Album", "02.mp3"), map[string]string{"TPE1": "Track Artist", "TPE2": "Album Artist", "TALB": "Album"})
	writeFile(t, filepath.Join(root, "Broken", "01.flac"), "not audio")
	writeFile(t, filepath.Join(root, "Docs", "notes.txt"), "text")
	writeMP3(t, filepath.Join(root, "Explo", "Weekly", "01.mp3"), tags("Someone", "Weekly"))

	tests := []struct {
		force bool
		want  []Album
	}{
		{false, []Album{
			{filepath.Join(root, "Korn", "Issues"), "Korn", "Issues"},
			{filepath.Join(root, "Mixed", "Album"), "Album Artist", "Album"},
		}},
		{true, []Album{
			{filepath.Join(root, "Korn", "Issues"), "Korn", "Issues"},
			{filepath.Join(root, "Korn", "Requiem"), "Korn", "Requiem"},
			{filepath.Join(root, "Mixed", "Album"), "Album Artist", "Album"},
		}},
	}
	for _, tt := range tests {
		got, err := Scan(root, []string{"explo"}, "cover.webp", tt.force)
		if err != nil {
			t.Fatal(err)
		}
		sort.Slice(got, func(i, j int) bool { return got[i].Path < got[j].Path })
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("force=%v:\n got %+v\nwant %+v", tt.force, got, tt.want)
		}
	}
}
