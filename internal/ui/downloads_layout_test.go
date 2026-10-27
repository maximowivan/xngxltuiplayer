package ui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"tuiplay/internal/download"
)

func TestDownloadsModalLongErrors(t *testing.T) {
	longErr := strings.Repeat("yt-dlp: ERROR: [soundcloud] rostiko5/birki: Unable to download JSON metadata: connection reset. ", 12)
	cases := map[string]string{
		"ascii":       longErr,
		"cyrillic":    strings.Repeat("Невозможно загрузить метаданные: соединение сброшено. ", 20),
		"invalid":     strings.Repeat(string([]byte{0xe0, 0xe1, 0xe2, 0xe3, 0xff, 0xfe}), 120),
		"tabs":        strings.Repeat("stage\tfailed\tUnable\tto\tdownload\t", 40),
		"newlines":    "first line\nsecond line\nthird line\nfourth line\nfifth line",
		"diamonds":    strings.Repeat("◆", 200),
		"replacement": strings.Repeat("�", 200),
		"escapes":     strings.Repeat("\x1b[31mERROR\x1b[0m\t", 40),
	}
	for name, detail := range cases {
		t.Run(name, func(t *testing.T) {
			for _, size := range [][2]int{{130, 40}, {110, 32}, {92, 28}, {84, 24}, {72, 20}, {60, 18}} {
				m, cleanup := testModel(t)
				m.modal = modalDownloads
				m.jobs = []download.Job{
					{ID: 1, URL: "https://soundcloud.com/rostiko5/birki", Title: "https://soundcloud.com/rostiko5/birki", Stage: "failed", Status: download.StatusFailed, Err: detail, Detail: detail},
					{ID: 2, URL: "https://soundcloud.com/rostiko5/birki", Title: "https://soundcloud.com/rostiko5/birki", Stage: "failed", Status: download.StatusFailed, Err: detail, Detail: detail},
					{ID: 3, URL: "https://soundcloud.com/rostiko5/birki", Title: "https://soundcloud.com/rostiko5/birki", Stage: "failed", Status: download.StatusFailed, Err: detail, Detail: detail},
				}
				m.downloadsCur = 0
				m.toast = "Download failed: " + truncateText(detail, 120)
				m.toastKind = toastError
				m.toastUntil = time.Now().Add(time.Hour)
				m.width, m.height = size[0], size[1]
				assertFrame(t, m, name)
				for _, line := range strings.Split(m.View(), "\n") {
					plain := ansi.Strip(line)
					if !utf8.ValidString(plain) {
						t.Fatalf("%s at %dx%d: invalid UTF-8 line", name, size[0], size[1])
					}
					if strings.ContainsAny(plain, "\t\r") {
						t.Fatalf("%s at %dx%d: control character in line %q", name, size[0], size[1], plain)
					}
				}
				cleanup()
			}
		})
	}
}
