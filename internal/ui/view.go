package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tuiplay/internal/model"
)

// View implements tea.Model.
func (m *Model) View() string {
	return paintBackground(m.renderView(), colBg)
}

func (m *Model) renderView() string {
	w, h := m.dims()
	if w < 54 || h < 16 {
		return m.renderTooSmall(w, h)
	}
	if m.loading || m.loadErr != nil {
		return m.renderLoading(w, h)
	}
	if m.modal != modalNone {
		return m.viewWithModal(w, h)
	}
	if m.coverMode {
		return m.renderCover(w, h)
	}

	_, _, contentH, _, _, _ := m.layout()
	header := m.renderHeader(w)
	content := m.renderContent(w, contentH)
	player := m.renderPlayer(w)
	hints := fitLine(m.renderHints(), w)
	toast := m.renderToastLine(w)

	return strings.Join([]string{header, content, player, hints, toast}, "\n")
}

func (m *Model) renderHeader(w int) string {
	left := gradientText(iconLogo+" xngxltuiplayer", colAccent, colAccent2)

	right := dimStyle.Render(m.viewLabel()) + faintStyle.Render(" · ") +
		dimStyle.Render(fmt.Sprintf("%d tracks", len(m.visible)))
	if m.search != "" {
		right += faintStyle.Render(" · ") + accent2Style.Render("filtered")
	}
	if active := m.mgr.ActiveCount(); active > 0 {
		right += faintStyle.Render(" · ") + accent2Style.Render(fmt.Sprintf("↓ %d", active))
	}
	return fitLine(padBetween(left, right, w), w)
}

func (m *Model) renderContent(w, height int) string {
	_, _, _, sidebarW, rightW, centerW := m.layout()
	panes := make([]string, 0, 3)
	if sidebarW > 0 {
		panes = append(panes, m.renderSidebar(sidebarW, height))
	}
	panes = append(panes, m.renderCenter(centerW, height))
	if rightW > 0 {
		panes = append(panes, m.renderRightPane(rightW, height))
	}
	return joinHorizontal(panes)
}

func joinHorizontal(panes []string) string {
	if len(panes) == 0 {
		return ""
	}
	split := make([][]string, len(panes))
	height := 0
	for i, pane := range panes {
		split[i] = strings.Split(pane, "\n")
		if len(split[i]) > height {
			height = len(split[i])
		}
	}
	var out strings.Builder
	for row := 0; row < height; row++ {
		for i, lines := range split {
			if i > 0 {
				out.WriteString(" ")
			}
			if row < len(lines) {
				out.WriteString(lines[row])
			}
		}
		if row < height-1 {
			out.WriteString("\n")
		}
	}
	return out.String()
}

func (m *Model) renderSidebar(width, height int) string {
	innerW := width - 4
	innerH := height - 2
	lines := make([]string, 0, innerH)

	names := m.playlists.Names()
	allActive := m.view == "all"
	allSelected := m.focus == focusSidebar && m.sidebarCursor == 0
	lines = append(lines, sidebarRow("All tracks", fmt.Sprintf("%d", len(m.tracks)), allActive, allSelected, false, innerW))
	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render(truncateText(fmt.Sprintf("PLAYLISTS  %d", len(names)), innerW)))

	rows := m.sidebarRows()
	if len(names) == 0 {
		lines = append(lines, dimStyle.Render(truncateText("No playlists", innerW)))
		lines = append(lines, dimStyle.Render(truncateText("P: create", innerW)))
	} else {
		start := m.sidebarOffset
		if start > len(names) {
			start = len(names)
		}
		end := minInt(len(names), start+rows)
		for i := start; i < end; i++ {
			name := names[i]
			count := 0
			if pl, ok := m.playlists.Get(name); ok {
				count = len(pl.Tracks)
			}
			active := strings.EqualFold(m.view, name)
			selected := m.focus == focusSidebar && m.sidebarCursor == i+1
			drop := m.dragActive && strings.EqualFold(m.dragTarget, name)
			lines = append(lines, sidebarRow("≡ "+name, fmt.Sprintf("%d", count), active, selected, drop, innerW))
		}
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	return renderBox("Library", lines, width, height, colFaint, colDim)
}

func sidebarRow(label, count string, active, selected, drop bool, width int) string {
	mark := " "
	if drop {
		mark = "→"
	} else if active {
		mark = "●"
	} else if selected {
		mark = "▸"
	}
	left := mark + " " + label
	plain := padBetween(left, count, width)
	switch {
	case drop:
		return dropStyle.Render(plain)
	case selected:
		return selectedStyle.Render(plain)
	case active:
		return playingStyle.Render(plain)
	default:
		return textStyle.Render(plain)
	}
}

func (m *Model) renderCenter(width, height int) string {
	innerW := width - 4
	innerH := height - 2
	lines := make([]string, 0, innerH)

	if m.searching {
		input := m.searchInput.View()
		if ansi.StringWidth(input) > innerW {
			input = ansi.Truncate(input, innerW, "…")
		}
		lines = append(lines, input)
	}

	rows := m.listRows()
	listLines := m.renderTrackRows(innerW, rows)
	lines = append(lines, listLines...)

	if len(lines) > innerH-1 {
		lines = lines[:maxInt(0, innerH-1)]
	}
	for len(lines) < innerH-1 {
		lines = append(lines, "")
	}

	footerLeft := fmt.Sprintf("%d tracks", len(m.visible))
	if m.search != "" {
		footerLeft = fmt.Sprintf("%d of %d tracks", len(m.visible), len(m.baseTracks()))
	}
	footerRight := ""
	if m.selectionCount() > 0 {
		footerLeft = fmt.Sprintf("%d selected of %d", m.selectionCount(), len(m.visible))
		footerRight = "x add · D delete"
	} else if m.current != nil && len(m.queue) > 0 {
		footerRight = fmt.Sprintf("queue %d/%d", m.queueIndex+1, len(m.queue))
	} else if len(m.visible) > 0 {
		footerRight = "enter to play"
	}
	lines = append(lines, dimStyle.Render(padBetween("  "+footerLeft, footerRight+"  ", innerW)))

	title := m.viewLabel()
	if m.search != "" {
		title += " · /" + truncateText(m.search, 24)
	}
	return renderBox(title, lines, width, height, colFaint, colAccent)
}

func (m *Model) renderTrackRows(width, rows int) []string {
	out := make([]string, 0, rows)
	if len(m.visible) == 0 {
		message := []string{}
		switch {
		case m.search != "":
			message = []string{
				"No tracks match " + truncateText("\""+m.search+"\"", maxInt(10, width-16)),
				"Press esc to clear the search.",
			}
		case m.view != "all":
			message = []string{
				"This playlist is empty.",
				"Open All tracks, select a track and press x.",
			}
		default:
			message = []string{
				"Your library is empty.",
				"Press a to paste a Spotify, SoundCloud or YouTube link.",
			}
		}
		top := maxInt(0, (rows-len(message)-2)/2)
		for i := 0; i < top; i++ {
			out = append(out, "")
		}
		out = append(out, centerLine(accentStyle.Render(iconLogo), width))
		out = append(out, "")
		for _, line := range message {
			out = append(out, centerLine(dimStyle.Render(truncateText(line, width)), width))
		}
		for len(out) < rows {
			out = append(out, "")
		}
		return out[:rows]
	}

	end := minInt(len(m.visible), m.listOffset+rows)
	for i := m.listOffset; i < end; i++ {
		t := m.visible[i]
		isCurrent := m.current != nil && trackKey(m.current.Path) == trackKey(t.Path)
		isCursor := i == m.cursor
		isMarked := m.isSelected(t.Path)

		cursor := " "
		if isCursor {
			cursor = "▸"
		} else if isMarked {
			cursor = "✓"
		}
		marker := " "
		if isCurrent {
			if m.loaded && !m.paused && !m.ended {
				marker = "▶"
			} else {
				marker = "‖"
			}
		}
		prefix := cursor + marker

		dur := model.FormatDuration(t.Duration)
		avail := width - 2 - 1 - ansi.StringWidth(dur)
		text := t.DisplayTitle()
		if artist := t.DisplayArtist(); artist != "" && avail > ansi.StringWidth(text)+4 {
			text += " · " + artist
		}
		if avail > 0 {
			text = truncateText(text, avail)
		} else {
			text = ""
		}
		plain := fitLine(prefix+" "+text, width-ansi.StringWidth(dur)-1)
		plain += " " + dur
		plain = fitLine(plain, width)

		switch {
		case isCursor || isMarked:
			out = append(out, selectedStyle.Render(plain))
		case isCurrent:
			out = append(out, playingStyle.Render(plain))
		default:
			out = append(out, textStyle.Render(plain))
		}
	}
	for len(out) < rows {
		out = append(out, "")
	}
	return out
}

func (m *Model) renderRightPane(width, height int) string {
	innerW := width - 4
	innerH := height - 2
	lines := make([]string, 0, innerH)

	artW, artH := m.rightArtSize(innerW, innerH)

	if artH > 0 {
		img := m.artImage
		if img == nil {
			img = placeholderImage(artW, artH*2)
		}
		artLines := renderArt(img, artW, artH)
		pad := maxInt(0, (innerW-artW)/2)
		for _, line := range artLines {
			lines = append(lines, fitLine(strings.Repeat(" ", pad)+line, innerW))
		}
		lines = append(lines, "")
	}

	if m.current == nil {
		lines = append(lines, centerLine(titleStyle.Render("Nothing playing"), innerW))
		lines = append(lines, centerLine(dimStyle.Render("Select a track and press enter"), innerW))
	} else {
		titleLines := wrapText(m.current.DisplayTitle(), innerW, 2)
		for _, line := range titleLines {
			lines = append(lines, titleStyle.Render(line))
		}
		lines = append(lines, accent2Style.Render(truncateText(m.current.DisplayArtist(), innerW)))
		album := m.current.DisplayAlbum()
		if y := strings.TrimSpace(m.current.Year); y != "" {
			album += " · " + y
		}
		ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(m.current.Path), "."))
		if ext == "" {
			ext = "AUDIO"
		}
		if strings.TrimSpace(album) != "" {
			album += " · " + ext
		} else {
			album = ext
		}
		lines = append(lines, dimStyle.Render(truncateText(album, innerW)))
		if m.artLoading {
			lines = append(lines, dimStyle.Render("loading cover…"))
		}
	}

	if innerH-len(lines) >= 3 {
		lines = append(lines, "")
		lines = append(lines, sectionStyle.Render("SPECTRUM")+dimStyle.Render("  live FFT"))
		// Let the bars use all remaining height and stay glued to the bottom
		// of the pane instead of leaving a growing gap underneath.
		rows := innerH - len(lines)
		if rows > 0 {
			lines = append(lines, renderSpectrum(m.spectrum, innerW, rows)...)
		}
	}

	for len(lines) < innerH {
		lines = append(lines, "")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	return renderBox("Now playing "+iconCover+" · c", lines, width, height, colFaint, colAccent2)
}

// rightArtSize returns the cover size that fits the now-playing pane.
func (m *Model) rightArtSize(innerW, innerH int) (artW, artH int) {
	artW = minInt(innerW, 40)
	artH = artW / 2
	if maxArt := innerH - 8; artH > maxArt {
		artH = maxArt
		artW = minInt(artW, artH*2)
	}
	if artH < 4 {
		return 0, 0
	}
	return artW, artH
}

// coverArtSize returns the cover dimensions and first row for cover mode.
func (m *Model) coverArtSize(w, h int) (artW, artH, artTop int) {
	available := h - 5
	if available < 4 {
		available = 4
	}
	artH = available
	artW = minInt(artH*2, w-4)
	artH = artW / 2
	if artH < 4 {
		artH, artW = 0, 0
	}
	slack := available - artH
	if slack < 0 {
		slack = 0
	}
	return artW, artH, 1 + slack/2
}

// renderCover draws a large version of the album art with a compact player.
func (m *Model) renderCover(w, h int) string {
	artW, artH, artTop := m.coverArtSize(w, h)
	blank := strings.Repeat(" ", w)
	lines := make([]string, 0, h)
	lines = append(lines, m.renderHeader(w))
	for len(lines) < artTop {
		lines = append(lines, blank)
	}

	if artW > 0 && artH > 0 {
		img := m.artImage
		if img == nil {
			img = placeholderImage(artW, artH*2)
		}
		art := renderArt(img, artW, artH)
		pad := maxInt(0, (w-artW)/2)
		for _, line := range art {
			lines = append(lines, fitLine(strings.Repeat(" ", pad)+line, w))
		}
	}
	for len(lines) < h-4 {
		lines = append(lines, blank)
	}

	title := "Nothing playing"
	icon := "■"
	iconStyle := faintStyle
	if m.current != nil {
		title = m.current.DisplayTitle()
		if artist := strings.TrimSpace(m.current.Artist); artist != "" {
			title += " — " + artist
		}
		icon = "‖"
		iconStyle = accent2Style
		if m.loaded && !m.paused && !m.ended {
			icon = "▶"
		}
	}
	titleLine := iconStyle.Render(icon) + " " + titleStyle.Render(truncateText(title, maxInt(10, w-6)))
	lines = append(lines, centerLine(titleLine, w))
	lines = append(lines, m.coverProgressLine(w))
	lines = append(lines, fitLine(m.renderCoverHints(), w))
	lines = append(lines, m.renderToastLine(w))

	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, blank)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderCoverHints() string {
	w, _ := m.dims()
	if w >= 80 {
		return hintLine(
			keyHint("c / esc", "close"),
			keyHint("space", "pause"),
			keyHint("← →", "seek"),
			keyHint("t", "theme"),
			keyHint("?", "help"),
			keyHint("q", "quit"),
		)
	}
	return hintLine(
		keyHint("c", "close"),
		keyHint("space", "pause"),
		keyHint("?", "help"),
	)
}

func (m *Model) renderPlayer(w int) string {
	innerW := maxInt(1, w-4)
	_, h := m.dims()
	_, rightW := paneWidths(w)
	lines := []string{
		m.playerTitleLine(innerW),
		m.playerProgressLine(innerW),
		m.playerTransportLine(innerW),
	}
	if h >= 20 {
		lines = append(lines, m.playerMetaLine(innerW))
	}
	if rightW == 0 && w >= 54 && h >= 24 {
		specW := maxInt(8, innerW-4)
		spec := renderSpectrum(m.spectrum, specW, 1)
		line := dimStyle.Render("FFT ")
		if len(spec) > 0 {
			line += spec[0]
		}
		lines = append(lines, fitLine(line, innerW))
	}
	return renderBox("Player", lines, w, m.playerHeight(), colFaint, colAccent2)
}

func (m *Model) playerTitleLine(innerW int) string {
	icon := "■"
	iconStyle := faintStyle
	left := dimStyle.Render("Nothing playing")
	if m.current != nil {
		if m.loaded && !m.paused && !m.ended {
			icon = iconPlay
		} else if m.paused || m.loaded {
			icon = iconPause
		}
		iconStyle = accent2Style
		title := titleStyle.Render(m.current.DisplayTitle())
		artist := dimStyle.Render(" — " + m.current.DisplayArtist())
		left = title + artist
	}
	left = iconStyle.Render(icon) + " " + left

	shuffleState := dimStyle.Render("off")
	if m.shuffle {
		shuffleState = accent2Style.Render("on")
	}
	repeatState := dimStyle.Render(m.repeat)
	if m.repeat != "off" {
		repeatState = accentStyle.Render(m.repeat)
	}
	volColor := colGreen
	if m.volume == 0 {
		volColor = colRed
	}
	barW := m.volumeBarWidth(innerW)
	vol := lipgloss.NewStyle().Foreground(volColor).Render(volumeBar(m.volume, barW))
	volGroup := dimStyle.Render("vol ") + keyStyle.Render("−") + vol + keyStyle.Render("+") +
		dimStyle.Render(fmt.Sprintf(" %3d%%", m.volume))
	right := keyStyle.Render(iconShuffle) + " " + shuffleState + faintStyle.Render("  ·  ") +
		keyStyle.Render(iconRepeat) + " " + repeatState + faintStyle.Render("  ·  ") + volGroup
	if innerW < 60 {
		right = volGroup
	}
	return fitLine(padBetween(left, right, innerW), innerW)
}

func (m *Model) playerProgressLine(innerW int) string {
	start, segW := m.playerCenterSegment(innerW)
	return m.progressLine(innerW, start, segW)
}

// coverProgressLine is used by the full-screen cover, where there is no player
// box and the bar can use the whole terminal width.
func (m *Model) coverProgressLine(w int) string {
	return m.progressLine(w, 0, w)
}

func (m *Model) progressLine(innerW, start, segW int) string {
	pos := m.currentPosition()
	dur := m.duration
	progress := 0.0
	if dur > 0 {
		progress = float64(pos) / float64(dur)
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	leftLabel := model.FormatDuration(pos)
	rightLabel := model.FormatDuration(dur)
	if m.current == nil {
		leftLabel = "--:--"
		rightLabel = "--:--"
	}
	timeW := maxInt(7, ansi.StringWidth(rightLabel))
	if segW < timeW*2+3 {
		barW := maxInt(1, segW)
		line := progressBar(progress, barW, m.current != nil && dur > 0)
		return fitLine(strings.Repeat(" ", maxInt(0, start))+line, innerW)
	}

	barW := segW - timeW*2 - 2
	if barW < 1 {
		barW = 1
	}
	bar := progressBar(progress, barW, m.current != nil && dur > 0)
	left := dimStyle.Render(fmt.Sprintf("%-*s", timeW, leftLabel))
	right := dimStyle.Render(fmt.Sprintf("%*s", timeW, rightLabel))
	line := left + " " + bar + " " + right
	if ansi.StringWidth(line) > segW {
		line = truncateText(line, segW)
	}
	if pad := segW - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return fitLine(strings.Repeat(" ", maxInt(0, start))+line, innerW)
}

func (m *Model) playerMetaLine(innerW int) string {
	var left string
	if m.current == nil {
		left = dimStyle.Render(fmt.Sprintf("offline · %d tracks in library", len(m.tracks)))
	} else {
		next := m.peekNextTitle()
		if next != "" {
			left = dimStyle.Render("next  ") + textStyle.Render(truncateText(next, maxInt(8, innerW/2)))
		} else {
			left = dimStyle.Render("album  ") + textStyle.Render(truncateText(m.current.DisplayAlbum(), maxInt(8, innerW/2)))
		}
	}
	right := ""
	if m.current != nil && len(m.queue) > 0 {
		right = fmt.Sprintf("%d/%d", m.queueIndex+1, len(m.queue))
	}
	if active := m.mgr.ActiveCount(); active > 0 {
		if right != "" {
			right += "  ·  "
		}
		right += fmt.Sprintf("↓ %d active", active)
	}
	if right == "" {
		right = fmt.Sprintf("%d tracks", len(m.tracks))
	}
	return fitLine(padBetween(left, right, innerW), innerW)
}

// playerCenterSegment returns a centered strip whose length matches the track
// list. Keeping the same width as the center pane makes the progress bar feel
// connected to the list, while centering it in the full player keeps the
// transport visually balanced against the wider now-playing pane.
func (m *Model) playerCenterSegment(innerW int) (start, width int) {
	_, _, _, _, _, centerW := m.layout()
	width = centerW - 4
	if width < 24 || width > innerW-4 {
		width = innerW - 8
	}
	if width < 8 {
		width = innerW
	}
	start = (innerW - width) / 2
	if start < 0 {
		start = 0
	}
	return
}

type transportLayout struct {
	prevStart, prevEnd int
	playStart, playEnd int
	nextStart, nextEnd int
}

func (m *Model) playerTransportLine(innerW int) string {
	_, line := m.transportLayout(innerW)
	return line
}

func (m *Model) transportLayout(innerW int) (transportLayout, string) {
	start, segW := m.playerCenterSegment(innerW)
	prevIcon := iconPrev
	nextIcon := iconNext
	playIcon := iconPlay
	if m.current != nil && m.loaded && !m.paused && !m.ended {
		playIcon = iconPause
	}

	gap := 3
	prevW := ansi.StringWidth(prevIcon)
	playW := ansi.StringWidth(playIcon)
	nextW := ansi.StringWidth(nextIcon)
	total := prevW + gap + playW + gap + nextW
	center := start + segW/2
	left := center - total/2
	if left < start {
		left = start
	}
	if left+total > start+segW {
		left = start + segW - total
	}
	if left < 0 {
		left = 0
	}
	if left+total > innerW {
		left = maxInt(0, innerW-total)
	}

	var out transportLayout
	out.prevStart = left
	out.prevEnd = left + prevW
	out.playStart = out.prevEnd + gap
	out.playEnd = out.playStart + playW
	out.nextStart = out.playEnd + gap
	out.nextEnd = out.nextStart + nextW

	prevStyle := keyStyle
	nextStyle := keyStyle
	playStyle := accent2Style
	if m.current == nil {
		playStyle = faintStyle
	}
	line := strings.Repeat(" ", left) +
		prevStyle.Render(prevIcon) + strings.Repeat(" ", gap) +
		playStyle.Render(playIcon) + strings.Repeat(" ", gap) +
		nextStyle.Render(nextIcon)
	return out, fitLine(line, innerW)
}

func (m *Model) peekNextTitle() string {
	if len(m.queue) == 0 {
		return ""
	}
	if m.shuffle {
		return "shuffle mode"
	}
	idx := m.queueIndex + 1
	if idx >= len(m.queue) {
		if m.repeat != "all" {
			return ""
		}
		idx = 0
	}
	if t, ok := m.findTrack(m.queue[idx]); ok {
		return t.DisplayTitle()
	}
	return ""
}

func progressBar(progress float64, width int, enabled bool) string {
	if width <= 0 {
		return ""
	}
	if !enabled {
		return faintStyle.Render(strings.Repeat("─", width))
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	filled := int(progress * float64(width))
	if filled > width {
		filled = width
	}
	if filled < width && progress > 0 {
		filled++
	}
	var b strings.Builder
	if filled > 0 {
		b.WriteString(accent2Style.Render(strings.Repeat("━", filled)))
	}
	if filled < width {
		b.WriteString(faintStyle.Render(strings.Repeat("─", width-filled)))
	}
	return b.String()
}

// volumeBarWidth scales the slider so it feels like a real control instead of
// a five-step indicator.
func (m *Model) volumeBarWidth(width int) int {
	switch {
	case width >= 120:
		return 30
	case width >= 100:
		return 24
	case width >= 80:
		return 16
	case width >= 60:
		return 10
	default:
		return 8
	}
}

type playerSuffixLayout struct {
	shuffleStart, shuffleEnd int
	repeatStart, repeatEnd   int
	volumeStart, volumeEnd   int
}

// playerSuffix mirrors the right-aligned controls in playerTitleLine so mouse
// hit testing stays correct for every width and icon set. The width is the
// player's inner width; add playerContentX to get terminal coordinates.
func (m *Model) playerSuffix(width int) playerSuffixLayout {
	barW := m.volumeBarWidth(width)
	pctW := ansi.StringWidth(fmt.Sprintf(" %3d%%", m.volume))
	volW := ansi.StringWidth("vol ") + 1 + barW + 1 + pctW
	volStart := width - volW
	barStart := volStart + ansi.StringWidth("vol ") + 1
	var out playerSuffixLayout
	out.volumeStart = barStart
	out.volumeEnd = barStart + barW
	if width < 60 {
		return out
	}

	shuffleState := "off"
	if m.shuffle {
		shuffleState = "on"
	}
	shuffleW := ansi.StringWidth(iconShuffle) + 1 + ansi.StringWidth(shuffleState)
	repeatW := ansi.StringWidth(iconRepeat) + 1 + ansi.StringWidth(m.repeat)
	total := shuffleW + 5 + repeatW + 5 + volW
	start := width - total
	out.shuffleStart = start
	out.shuffleEnd = start + shuffleW
	out.repeatStart = out.shuffleEnd + 5
	out.repeatEnd = out.repeatStart + repeatW

	if out.volumeStart < 0 {
		out.volumeStart = 0
	}
	if out.volumeEnd > width {
		out.volumeEnd = width
	}
	if out.volumeEnd < out.volumeStart {
		out.volumeEnd = out.volumeStart
	}
	return out
}

// playerContentX is the terminal column where the player box inner content
// starts (left border + one cell of padding).
const playerContentX = 2

// volumeBarBounds returns the terminal x range of the volume bar in the player
// title line.
func (m *Model) volumeBarBounds(width int) (start, end int) {
	layout := m.playerSuffix(maxInt(1, width-4))
	return playerContentX + layout.volumeStart, playerContentX + layout.volumeEnd
}

// playerProgressGeometry returns the terminal x range of the seek bar.
func (m *Model) playerProgressGeometry(width int) (start, barWidth int) {
	innerW := maxInt(1, width-4)
	segStart, segW := m.playerCenterSegment(innerW)
	rightLabel := model.FormatDuration(m.duration)
	timeW := maxInt(7, ansi.StringWidth(rightLabel))
	if segW < timeW*2+3 {
		return playerContentX + segStart, maxInt(1, segW)
	}
	barWidth = segW - timeW*2 - 2
	if barWidth < 1 {
		barWidth = 1
	}
	return playerContentX + segStart + timeW + 1, barWidth
}

// coverProgressGeometry returns the terminal x range of the seek bar in cover
// mode.
func (m *Model) coverProgressGeometry(width int) (start, barWidth int) {
	rightLabel := model.FormatDuration(m.duration)
	timeW := maxInt(7, ansi.StringWidth(rightLabel))
	if width < timeW*2+3 {
		return 0, maxInt(1, width)
	}
	barWidth = width - timeW*2 - 2
	if barWidth < 1 {
		barWidth = 1
	}
	return timeW + 1, barWidth
}

// playerTransportBounds returns the terminal x ranges of the transport
// controls.
func (m *Model) playerTransportBounds(width int) transportLayout {
	innerW := maxInt(1, width-4)
	layout, _ := m.transportLayout(innerW)
	layout.prevStart += playerContentX
	layout.prevEnd += playerContentX
	layout.playStart += playerContentX
	layout.playEnd += playerContentX
	layout.nextStart += playerContentX
	layout.nextEnd += playerContentX
	return layout
}

var volumePartials = []rune("▏▎▍▌▋▊▉")

func volumeBar(volume, width int) string {
	if width <= 0 {
		return ""
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	// Work in eighths of a cell so the slider looks continuous even though the
	// mouse resolves whole cells.
	units := volume * width
	full := units / 100
	partial := (units % 100) * (len(volumePartials) + 1) / 100
	var b strings.Builder
	b.WriteString(strings.Repeat("▮", full))
	empty := width - full
	if empty > 0 && partial > 0 {
		b.WriteRune(volumePartials[partial-1])
		empty--
	}
	b.WriteString(strings.Repeat("▯", empty))
	return b.String()
}

func (m *Model) renderHints() string {
	w, _ := m.dims()
	if m.searching {
		return hintLine(
			keyHint("type", "filter"),
			keyHint("↑↓", "select"),
			keyHint("enter", "play"),
			keyHint("esc", "clear"),
		)
	}
	if m.selectionCount() > 0 {
		if w >= 96 {
			return hintLine(
				keyHint("ctrl+click", "toggle"),
				keyHint("drag", "to playlist"),
				keyHint("x", "add"),
				keyHint("D", "delete"),
				keyHint("esc", "clear"),
			)
		}
		return hintLine(
			keyHint("x", "add"),
			keyHint("D", "delete"),
			keyHint("esc", "clear"),
		)
	}
	if m.focus == focusSidebar {
		if w >= 96 {
			return hintLine(
				keyHint("↑↓", "playlists"),
				keyHint("enter", "open"),
				keyHint("tab", "tracks"),
				keyHint("P", "new"),
				keyHint("D", "delete"),
				keyHint("a", "add link"),
				keyHint("?", "help"),
				keyHint("q", "quit"),
			)
		}
		if w >= 72 {
			return hintLine(
				keyHint("↑↓", "move"),
				keyHint("enter", "open"),
				keyHint("tab", "tracks"),
				keyHint("P", "new"),
				keyHint("?", "help"),
				keyHint("q", "quit"),
			)
		}
		return hintLine(
			keyHint("↑↓", "move"),
			keyHint("enter", "open"),
			keyHint("tab", "tracks"),
			keyHint("?", "help"),
		)
	}
	if w >= 104 {
		return hintLine(
			keyHint("enter", "play"),
			keyHint("space", "pause"),
			keyHint("/", "search"),
			keyHint("a", "add link"),
			keyHint("P", "playlist"),
			keyHint("x", "add"),
			keyHint("d", "downloads"),
			keyHint("?", "help"),
			keyHint("q", "quit"),
		)
	}
	if w >= 80 {
		return hintLine(
			keyHint("enter", "play"),
			keyHint("space", "pause"),
			keyHint("/", "search"),
			keyHint("a", "add"),
			keyHint("d", "downloads"),
			keyHint("?", "help"),
		)
	}
	return hintLine(
		keyHint("enter", "play"),
		keyHint("space", "pause"),
		keyHint("/", "search"),
		keyHint("?", "help"),
	)
}

func (m *Model) renderToastLine(w int) string {
	if m.toast == "" {
		return strings.Repeat(" ", w)
	}
	return renderToast(m.toast, m.toastKind, w)
}

func (m *Model) viewWithModal(w, h int) string {
	_, _, contentH, _, _, _ := m.layout()
	header := m.renderHeader(w)
	modal := m.renderModal(w, contentH)
	content := placeBlock(modal, w, contentH)
	player := m.renderPlayer(w)
	hints := fitLine(m.renderModalHints(w), w)
	toast := m.renderToastLine(w)
	return strings.Join([]string{header, content, player, hints, toast}, "\n")
}

func (m *Model) renderModalHints(w int) string {
	switch m.modal {
	case modalAddLink, modalNewPlaylist:
		return hintLine(
			keyHint("enter", "confirm"),
			keyHint("esc", "cancel"),
			keyHint("ctrl+v", "paste"),
		)
	case modalAddToPlaylist:
		return hintLine(
			keyHint("↑↓", "choose"),
			keyHint("enter", "add"),
			keyHint("esc", "cancel"),
		)
	case modalConfirmRemoveFromPlaylist:
		return hintLine(
			keyHint("y / enter", "remove"),
			keyHint("d", "delete from disk"),
			keyHint("esc", "cancel"),
		)
	case modalConfirmDeleteTrack, modalConfirmDeletePlaylist:
		return hintLine(
			keyHint("y / enter", "delete"),
			keyHint("esc", "cancel"),
		)
	case modalDownloads:
		if w < 80 {
			return hintLine(
				keyHint("↑↓", "select"),
				keyHint("c", "cancel"),
				keyHint("C", "clear"),
				keyHint("esc", "close"),
			)
		}
		return hintLine(
			keyHint("↑↓", "select"),
			keyHint("c", "cancel"),
			keyHint("C", "clear finished"),
			keyHint("a", "add link"),
			keyHint("esc", "close"),
		)
	case modalHelp:
		return hintLine(
			keyHint("↑↓", "scroll"),
			keyHint("esc", "close"),
			keyHint("?", "close"),
		)
	}
	return ""
}

func (m *Model) renderLoading(w, h int) string {
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	frame := frames[m.spinner%len(frames)]

	logo := gradientText(iconLogo+" xngxltuiplayer", colAccent, colAccent2)
	var body string
	if m.loadErr != nil {
		body = logo + "\n\n" +
			errorStyle.Render("Could not load the library") + "\n\n" +
			dimStyle.Render(truncateText(m.loadErr.Error(), maxInt(20, w-12))) + "\n\n" +
			hintLine(keyHint("r", "retry"), keyHint("q", "quit"))
	} else {
		body = logo + "\n\n" +
			accent2Style.Render(string(frame)) + "  " + textStyle.Render("Scanning your library…") + "\n\n" +
			dimStyle.Render(truncateText(m.cfg.MusicDir, maxInt(20, w-12))) + "\n" +
			dimStyle.Render("The first scan may take a moment.")
	}
	return placeBlock(body, w, h)
}

func (m *Model) renderTooSmall(w, h int) string {
	body := gradientText(iconLogo+" xngxltuiplayer", colAccent, colAccent2) + "\n\n" +
		textStyle.Render("Terminal too small") + "\n" +
		dimStyle.Render(fmt.Sprintf("%d×%d — need at least 54×16", w, h))
	return placeBlock(body, w, h)
}

func padBetween(left, right string, width int) string {
	lw := ansi.StringWidth(left)
	rw := ansi.StringWidth(right)
	if lw+rw+1 > width {
		if rw >= width {
			return truncateText(right, width)
		}
		left = truncateText(left, width-rw-1)
		lw = ansi.StringWidth(left)
	}
	gap := width - lw - rw
	if gap < 0 {
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right
}

func placeBlock(block string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(block, "\n")
	out := make([]string, height)
	for i := range out {
		out[i] = strings.Repeat(" ", width)
	}
	top := (height - len(lines)) / 2
	if top < 0 {
		top = 0
	}
	for i, line := range lines {
		row := top + i
		if row >= height {
			break
		}
		w := ansi.StringWidth(line)
		left := (width - w) / 2
		if left < 0 {
			left = 0
		}
		out[row] = fitLine(strings.Repeat(" ", left)+line, width)
	}
	return strings.Join(out, "\n")
}
