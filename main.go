// Command xngxltuiplayer is a small offline music player with a terminal UI.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tuiplay/internal/applog"
	"tuiplay/internal/audio"
	"tuiplay/internal/config"
	"tuiplay/internal/download"
	"tuiplay/internal/instance"
	"tuiplay/internal/library"
	"tuiplay/internal/playlist"
	"tuiplay/internal/presence"
	"tuiplay/internal/session"
	"tuiplay/internal/stats"
	"tuiplay/internal/terminal"
	"tuiplay/internal/ui"
	"tuiplay/internal/updater"
)

const appName = "xngxltuiplayer"

// version is overridden at build time:
//
//	go build -ldflags "-X main.version=0.2.0"
var version = "0.2.1-dev"

func main() {
	defer recoverPanic()

	var (
		musicDir  = flag.String("music", "", "music library folder (default: ./music next to the executable)")
		dataDir   = flag.String("data", "", "data folder for playlists, cache and session (default: ./data)")
		themeName = flag.String("theme", "", "color theme: catppuccin, tokyonight, gruvbox, nord, dracula, rosepine, ember")
		ascii     = flag.Bool("ascii", false, "use plain-text UI labels instead of Unicode icons")
		icons     = flag.String("icons", "auto", "icon mode: auto, unicode or ascii (auto falls back on classic consoles)")
		discordID = flag.String("discord", "", "Discord application client id for Rich Presence")
		check     = flag.Bool("check", false, "check directories, external tools and audio output, then exit")
		update    = flag.Bool("update", false, "download and install the latest release, then restart")
		showVer   = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("%s %s\n", appName, version)
		return
	}
	iconMode, err := parseIconMode(*icons, *ascii)
	if err != nil {
		fatal(err)
	}

	cfg, err := config.Load(*musicDir, *dataDir)
	if err != nil {
		fatal(err)
	}
	if err := applog.Init(cfg.DataDir); err != nil {
		fmt.Fprintln(os.Stderr, appName+": could not open log:", err)
	} else {
		applog.Printf("%s %s starting", appName, version)
		applog.Printf("music=%s data=%s", cfg.MusicDir, cfg.DataDir)
	}

	tools := download.FindTools()
	termInfo := terminal.Detect()
	asciiMode := resolveASCIITerminal(iconMode, termInfo)
	applog.Printf("terminal=%s elevated=%v font=%q cp=%d console=%q unicode=%v ascii=%v",
		termInfo.Program, termInfo.Elevated, termInfo.FontFace, termInfo.OutputCP, termInfo.ConsoleClass, termInfo.Unicode, asciiMode)

	presenceCfg := presence.Load(cfg.DataDir)
	if id := strings.TrimSpace(*discordID); id != "" {
		presenceCfg.Enabled = true
		presenceCfg.ClientID = id
	}
	if presenceCfg.ActivityType <= 0 {
		presenceCfg.ActivityType = 2
	}

	if *check {
		ok := runCheck(cfg, tools, presenceCfg, termInfo, asciiMode)
		applog.Close()
		if !ok {
			os.Exit(1)
		}
		return
	}
	if strings.TrimSpace(*discordID) != "" {
		if err := presence.Save(cfg.DataDir, presenceCfg); err != nil {
			fmt.Fprintln(os.Stderr, appName+": could not save Discord settings:", err)
		}
	}

	lock, err := instance.Acquire(cfg.DataDir)
	if err != nil {
		if err == instance.ErrAlreadyRunning {
			fatal(fmt.Errorf("another player is already using %s; close it first", cfg.DataDir))
		}
		fatal(fmt.Errorf("lock data folder: %w", err))
	}
	defer lock.Release()

	if *update {
		runUpdate(cfg)
		return
	}

	state := session.Load(cfg.DataDir)
	if strings.TrimSpace(*themeName) != "" {
		state.Theme = *themeName
	}
	playlists, err := playlist.Load(cfg.DataDir)
	if err != nil {
		applog.Printf("load playlists: %v", err)
		fmt.Fprintln(os.Stderr, appName+": playlists were damaged:", err)
		fmt.Fprintln(os.Stderr, appName+": starting with an empty playlist list; the damaged file was kept")
	}

	lib := library.New(cfg.MusicDir, cfg.DataDir)
	trackStats := stats.Load(cfg.DataDir)
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

	ui.SetASCIIIcons(asciiMode)
	ui.SetStartupNote(termInfo.StartupNote(asciiMode))
	model := ui.New(cfg, lib, playlists, trackStats, engine, mgr, state)
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
	applog.Printf("stopped")
	applog.Close()
}

func parseIconMode(value string, ascii bool) (string, error) {
	if ascii {
		return "ascii", nil
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto", nil
	case "unicode", "ascii":
		return strings.ToLower(strings.TrimSpace(value)), nil
	default:
		return "", fmt.Errorf("invalid -icons value %q: expected auto, unicode or ascii", value)
	}
}

func resolveASCIITerminal(mode string, info terminal.Info) bool {
	switch mode {
	case "ascii":
		return true
	case "unicode":
		return false
	default:
		return info.UseASCII()
	}
}

func recoverPanic() {
	r := recover()
	if r == nil {
		return
	}
	applog.Printf("panic: %v\n%s", r, debug.Stack())
	fmt.Fprint(os.Stderr, "\x1b[?1049l\x1b[?25h\x1b[0m")
	fmt.Fprintf(os.Stderr, "%s: unexpected error: %v\n", appName, r)
	fmt.Fprintf(os.Stderr, "details: %s\n", applog.Describe())
	applog.Close()
	os.Exit(2)
}

func runUpdate(cfg config.Config) {
	if runtime.GOOS != "windows" {
		fmt.Println("automatic updates are available for the Windows build only")
		fmt.Println("download the latest release from https://github.com/maximowivan/xngxltuiplayer/releases")
		return
	}
	if exe, err := os.Executable(); err == nil && strings.Contains(strings.ToLower(exe), "go-build") {
		fatal(fmt.Errorf("--update must be run from the built xngxltuiplayer.exe, not via go run"))
	}

	fmt.Printf("%s %s\n", appName, version)
	ctx := context.Background()
	rel, err := updater.Latest(ctx)
	if err != nil {
		fatal(fmt.Errorf("check latest release: %w", err))
	}
	if !updater.IsNewer(rel.TagName, version) {
		fmt.Printf("no newer release (latest %s, current %s)\n", rel.TagName, version)
		applog.Close()
		return
	}

	fmt.Printf("latest release: %s\n", rel.TagName)
	fmt.Println("downloading and verifying SHA256...")
	stage, err := updater.Stage(ctx, cfg.DataDir, rel)
	if err != nil {
		fatal(fmt.Errorf("download update: %w", err))
	}
	fmt.Println("verified, installing...")
	if err := updater.Apply(stage, cfg.BaseDir); err != nil {
		fatal(fmt.Errorf("install update: %w", err))
	}
	fmt.Println("update staged; the player will restart")
	applog.Printf("update staged from %s", rel.TagName)
	applog.Close()
}

func runCheck(cfg config.Config, tools download.Tools, presenceCfg presence.Config, termInfo terminal.Info, asciiMode bool) bool {
	ok := true
	fmt.Printf("%s %s\n\n", appName, version)

	fmt.Println("Terminal")
	for _, line := range termInfo.Summary() {
		fmt.Println(line)
	}
	iconMode := "unicode"
	if asciiMode {
		iconMode = "ascii"
	}
	fmt.Printf("  icon mode    %s\n", iconMode)

	fmt.Println("\nDirectories")
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
	fmt.Printf("  log          %s\n", applog.Describe())

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
	applog.Printf("fatal: %s", msg)
	fmt.Fprintln(os.Stderr, appName+":", msg)
	applog.Close()
	os.Exit(1)
}
