package download

import (
	"bytes"
	"encoding/binary"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dhowden/tag"
)

func TestSafeProcessRecovers(t *testing.T) {
	path, warning, err := safeProcess(func() (string, string, error) {
		panic("bad parser state")
	})
	if err == nil || !strings.Contains(err.Error(), "internal download error") {
		t.Fatalf("err = %v, want internal download error", err)
	}
	if path != "" || warning != "" {
		t.Fatalf("path=%q warning=%q on panic", path, warning)
	}
}

func TestEnqueueReportsAllMissingTools(t *testing.T) {
	base := t.TempDir()
	mgr := NewManager(Options{
		MusicDir: filepath.Join(base, "music"),
		DataDir:  filepath.Join(base, "data"),
		Tools:    Tools{},
	})
	defer mgr.Close()

	_, err := mgr.Enqueue("https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err == nil {
		t.Fatal("expected missing tools error")
	}
	if !strings.Contains(err.Error(), "yt-dlp") || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("error does not mention both tools: %v", err)
	}
}

func TestDirectDownloadPipeline(t *testing.T) {
	tools := FindTools()
	if tools.FFmpeg == "" {
		t.Skip("ffmpeg is not installed")
	}

	base := t.TempDir()
	musicDir := filepath.Join(base, "music")
	dataDir := filepath.Join(base, "data")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}

	wav := testWAV(0.6, 440)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wav)
	}))
	defer server.Close()

	mgr := NewManager(Options{
		MusicDir: musicDir,
		DataDir:  dataDir,
		TempDir:  filepath.Join(dataDir, "tmp"),
		Tools:    tools,
	})
	defer mgr.Close()

	job, err := mgr.Enqueue(server.URL + "/aurora-night-drive.wav")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var final Job
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		jobs := mgr.Jobs()
		if len(jobs) == 1 && jobs[0].Status.Terminal() {
			final = jobs[0]
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	if final.ID == 0 {
		t.Fatal("download did not finish in time")
	}
	if final.Status != StatusDone {
		t.Fatalf("status = %v (%s), err = %q", final.Status, final.Detail, final.Err)
	}
	if final.Path == "" {
		t.Fatal("finished job has no path")
	}
	if filepath.Dir(final.Path) != musicDir {
		t.Fatalf("saved outside music dir: %s", final.Path)
	}
	if filepath.Ext(final.Path) != ".mp3" {
		t.Fatalf("unexpected extension: %s", final.Path)
	}
	info, err := os.Stat(final.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 1000 {
		t.Fatalf("output too small: %d bytes", info.Size())
	}

	f, err := os.Open(final.Path)
	if err != nil {
		t.Fatal(err)
	}
	md, err := tag.ReadFrom(f)
	_ = f.Close()
	if err != nil {
		t.Fatalf("read tags: %v", err)
	}
	if md.Title() == "" {
		t.Fatal("downloaded file has no title tag")
	}
	if job.ID != final.ID {
		t.Fatalf("job id changed: %d -> %d", job.ID, final.ID)
	}
}

func testWAV(seconds, freq float64) []byte {
	const sampleRate = 48000
	samples := int(seconds * sampleRate)
	dataSize := samples * 2 * 2

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(2))
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*4))
	binary.Write(&buf, binary.LittleEndian, uint16(4))
	binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	for i := 0; i < samples; i++ {
		v := int16(math.Sin(2*math.Pi*freq*float64(i)/sampleRate) * 12000)
		binary.Write(&buf, binary.LittleEndian, v)
		binary.Write(&buf, binary.LittleEndian, v)
	}
	return buf.Bytes()
}
