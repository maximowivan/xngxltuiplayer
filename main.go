// Command xngxltuiplayer is a small offline music player with a terminal UI.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/audio"
	"tuiplay/internal/config"
	"tuiplay/internal/download"
	"tuiplay/internal/library"
	"tuiplay/internal/playlist"
	"tuiplay/internal/presence"
	"tuiplay/internal/session"
	"tuiplay/internal/ui"
)

const (
	appName = "xngxltuiplayer"
	version = "0.1.0"
)

func main() {
	var (
		musicDir  = flag.String("music", "", "music library folder (default: ./music next to the executable)")
		dataDir   = flag.String("data", "", "data folder for playlists, cache and session (default: ./data)")
		themeName = flag.String("theme", "", "color theme: catppuccin, tokyonight, gruvbox, nord, dracula, rosepine, ember")
		ascii     = flag.Bool("ascii", false, "use plain-text UI labels instead of Unicode icons")
		discordID = flag.String("discord", "", "Discord application client id for Rich Presence")
		check     = flag.Bool("check", false, "check directories, external tools and audio output, then exit")
		showVer   = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("%s %s\n", appName, version)
		return
	}

	cfg, err := config.Load(*musicDir, *dataDir)
	if err != nil {
		fatal(err)
	}
	tools := download.FindTools()
	presenceCfg := presence.Load(cfg.DataDir)
	if id := strings.TrimSpace(*discordID); id != "" {
		presenceCfg.Enabled = true
		presenceCfg.ClientID = id
	}
	if presenceCfg.ActivityType <= 0 {
		presenceCfg.ActivityType = 2
	}

	if *check {
		if !runCheck(cfg, tools, presenceCfg) {
			os.Exit(1)
		}
		return
	}
	if strings.TrimSpace(*discordID) != "" {
		if err := presence.Save(cfg.DataDir, presenceCfg); err != nil {
			fmt.Fprintln(os.Stderr, appName+": could not save Discord settings:", err)
		}
	}

	state := session.Load(cfg.DataDir)
	if strings.TrimSpace(*themeName) != "" {
		state.Theme = *themeName
	}
	playlists, err := playlist.Load(cfg.DataDir)
	if err != nil {
		fatal(fmt.Errorf("load playlists: %w", err))
	}

	lib := library.New(cfg.MusicDir, cfg.DataDir)
	engine := audio.New(float64(state.Volume) / 100)
	mgr := download.NewManager(download.Options{
		MusicDir:    cfg.MusicDir,
		DataDir:     cfg.DataDir,
		TempDir:     cfg.TempDir,
		Tools:       tools,
		CookiesFile: filepath.Join(cfg.DataDir, "cookies.txt"),
	})

	var rpc *presence.Client
	if presenceCfg.Configured() {
		rpc = presence.New(presenceCfg)
	}

	ui.SetASCIIIcons(*ascii)
	model := ui.New(cfg, lib, playlists, engine, mgr, state)
	if rpc != nil {
		model.SetPresence(rpc)
	}
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, runErr := program.Run()

	model.SaveSession()
	engine.Close()
	mgr.Close()
	if rpc != nil {
		rpc.Close()
	}

	if runErr != nil {
		fatal(runErr)
	}
}

func runCheck(cfg config.Config, tools download.Tools, presenceCfg presence.Config) bool {
	ok := true
	fmt.Printf("%s %s\n\n", appName, version)

	fmt.Println("Directories")
	dirs := []struct {
		name string
		path string
	}{
		{"music", cfg.MusicDir},
		{"data", cfg.DataDir},
		{"covers", cfg.CoversDir},
		{"temp", cfg.TempDir},
	}
	for _, d := range dirs {
		status := "ok"
		if err := config.Writable(d.path); err != nil {
			status = "NOT WRITABLE: " + err.Error()
			ok = false
		}
		fmt.Printf("  %-8s %s  [%s]\n", d.name, d.path, status)
	}

	fmt.Println("\nExternal tools")
	toolChecks := []struct {
		name     string
		path     string
		args     []string
		required bool
	}{
		{"yt-dlp", tools.YtDlp, []string{"--version"}, true},
		{"ffmpeg", tools.FFmpeg, []string{"-version"}, true},
		{"ffprobe", tools.FFprobe, []string{"-version"}, false},
	}
	for _, t := range toolChecks {
		ver := download.Version(context.Background(), t.path, t.args...)
		location := t.path
		if location == "" {
			location = "not found on PATH"
		}
		fmt.Printf("  %-8s %s\n           %s\n", t.name, ver, location)
		if t.required && t.path == "" {
			ok = false
			fmt.Printf("           %s\n", download.InstallHint(t.name))
		} else if !t.required && t.path == "" {
			fmt.Println("           optional: durations will be filled in after playback")
		}
	}

	fmt.Println("\nDiscord Rich Presence")
	switch {
	case presenceCfg.Configured():
		fmt.Printf("  enabled - client id %s, activity type %d\n", presenceCfg.ClientID, presenceCfg.ActivityType)
		if err := presence.Probe(presenceCfg); err != nil {
			fmt.Printf("  connection: failed - %s\n", presence.DescribeError(err))
			fmt.Println("  Discord must be running, and the player must have the same")
			fmt.Println("  or higher privileges (run Discord normally, or run the player")
			fmt.Println("  as administrator)")
		} else {
			fmt.Println("  connection: ok - Discord IPC handshake succeeded")
		}
	case presenceCfg.ClientID != "" && presenceCfg.Enabled:
		fmt.Println("  client id looks invalid — expected a numeric Discord application id")
	case presenceCfg.ClientID != "":
		fmt.Println("  client id is set, but the integration is disabled")
	default:
		fmt.Println("  not configured — create data\\discord.json or start with -discord <client-id>")
	}

	fmt.Println("\nAudio output")
	if err := audio.Check(); err != nil {
		fmt.Printf("  error: %v\n", err)
		ok = false
	} else {
		fmt.Println("  ok — 48 kHz stereo output available")
	}

	fmt.Println("\nSession")
	cookies := filepath.Join(cfg.DataDir, "cookies.txt")
	if info, err := os.Stat(cookies); err == nil && info.Size() > 0 {
		fmt.Println("  cookies.txt found — yt-dlp will use it")
	} else {
		fmt.Println("  no cookies.txt (only needed for restricted sources)")
	}

	fmt.Println()
	if ok {
		fmt.Println("status: ready")
	} else {
		fmt.Println("status: missing required components (see above)")
	}
	return ok
}

func fatal(err error) {
	msg := strings.TrimSpace(err.Error())
	fmt.Fprintln(os.Stderr, appName+":", msg)
	os.Exit(1)
}
