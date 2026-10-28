package updater

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v0.2.0", "0.2.0", false},
		{"v0.2.0", "0.2.0-dev", true},
		{"v0.2.1", "0.2.0", true},
		{"v0.2.0", "0.3.0", false},
		{"v0.3.0", "0.2.9", true},
		{"v1.0.0", "0.9.9", true},
		{"v0.2.0-beta", "0.2.0", false},
	}
	for _, tc := range cases {
		if got := IsNewer(tc.latest, tc.current); got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestVersionParts(t *testing.T) {
	got := versionParts("v1.2.3-beta+meta")
	if got != [3]int{1, 2, 3} {
		t.Fatalf("versionParts = %v", got)
	}
}

func TestWindowsAssetsAcceptsBothNames(t *testing.T) {
	rel := Release{Assets: []Asset{
		{Name: "xngxltuiplayer-0.2.0-windows-amd64.zip", BrowserDownloadURL: "old"},
		{Name: "xngxltuiplayer-0.2.0-windows-amd64.zip.sha256", BrowserDownloadURL: "old-sha"},
		{Name: "xngxltuiplayer-v0.2.0-windows-amd64.zip", BrowserDownloadURL: "new"},
		{Name: "xngxltuiplayer-v0.2.0-windows-amd64.zip.sha256", BrowserDownloadURL: "new-sha"},
	}}
	zip, sha, err := rel.WindowsAssets()
	if err != nil {
		t.Fatal(err)
	}
	if zip.BrowserDownloadURL != "new" || sha.BrowserDownloadURL != "new-sha" {
		t.Fatalf("v-prefixed assets should win: zip=%+v sha=%+v", zip, sha)
	}
}

func TestWindowsAssetsAcceptsUnprefixedName(t *testing.T) {
	rel := Release{Assets: []Asset{
		{Name: "xngxltuiplayer-0.2.0-windows-amd64.zip", BrowserDownloadURL: "old"},
		{Name: "xngxltuiplayer-0.2.0-windows-amd64.zip.sha256", BrowserDownloadURL: "old-sha"},
	}}
	zip, sha, err := rel.WindowsAssets()
	if err != nil {
		t.Fatal(err)
	}
	if zip.Name == "" || sha.Name == "" {
		t.Fatalf("unprefixed assets not found: zip=%+v sha=%+v", zip, sha)
	}
}

func TestStageWithProgressLocalServer(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	w, err := zw.Create("xngxltuiplayer.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("fake exe")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive.Bytes())
	shaBody := []byte(hex.EncodeToString(sum[:]) + "  xngxltuiplayer-v9.9.9-windows-amd64.zip\n")

	mux := http.NewServeMux()
	mux.HandleFunc("/update.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(archive.Len()))
		_, _ = w.Write(archive.Bytes())
	})
	mux.HandleFunc("/update.zip.sha256", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(shaBody)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	rel := Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: "xngxltuiplayer-v9.9.9-windows-amd64.zip", BrowserDownloadURL: server.URL + "/update.zip", Size: int64(archive.Len())},
			{Name: "xngxltuiplayer-v9.9.9-windows-amd64.zip.sha256", BrowserDownloadURL: server.URL + "/update.zip.sha256"},
		},
	}
	var phases []string
	stage, err := StageWithProgress(t.Context(), t.TempDir(), rel, func(p Progress) {
		if len(phases) == 0 || phases[len(phases)-1] != p.Phase {
			phases = append(phases, p.Phase)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stage, "xngxltuiplayer.exe")); err != nil {
		t.Fatalf("staged exe missing: %v", err)
	}
	want := []string{"download", "verify", "extract", "ready"}
	if fmt.Sprint(phases) != fmt.Sprint(want) {
		t.Fatalf("phases = %v, want %v", phases, want)
	}
}

func TestStageCleansUpOnSHAFailure(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	_, _ = zw.Create("xngxltuiplayer.exe")
	_ = zw.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/update.zip", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive.Bytes())
	})
	mux.HandleFunc("/update.zip.sha256", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  bad\n"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	rel := Release{
		TagName: "v9.9.8",
		Assets: []Asset{
			{Name: "xngxltuiplayer-v9.9.8-windows-amd64.zip", BrowserDownloadURL: server.URL + "/update.zip"},
			{Name: "xngxltuiplayer-v9.9.8-windows-amd64.zip.sha256", BrowserDownloadURL: server.URL + "/update.zip.sha256"},
		},
	}
	dataDir := t.TempDir()
	if _, err := Stage(t.Context(), dataDir, rel); err == nil {
		t.Fatal("expected a SHA256 error")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "tmp", "update-v9.9.8")); !os.IsNotExist(err) {
		t.Fatalf("failed stage was not cleaned up: %v", err)
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("v0.2.0 beta"); got != "v0.2.0-beta" {
		t.Fatalf("sanitize = %q", got)
	}
}
