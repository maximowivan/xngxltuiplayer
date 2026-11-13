package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tuiplay/internal/audio"
	"tuiplay/internal/model"
)

// View implements tea.Model.
func (m *Model) View() string {
	w, h := m.dims()
	return paintBackground(sanitizeFrame(m.renderView(), w, h), colBg)
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
	if m.coverMode {
		return m.renderCoverHeader(w)
	}
	left := gradientText(iconLogo+" xngxltuiplayer", colAccent, colAccent2)

	right := dimStyle.Render(m.viewLabel()) + faintStyle.Render(currentGlyphs.Separator) +
		dimStyle.Render(fmt.Sprintf("%d tracks", len(m.visible)))
	if m.search != "" {
		right += faintStyle.Render(currentGlyphs.Separator) + accent2Style.Render("filtered")
	}
	if active := m.mgr.ActiveCount(); active > 0 {
		right += faintStyle.Render(currentGlyphs.Separator) + accent2Style.Render(fmt.Sprintf("%s %d", currentGlyphs.Down, active))
	}
	return fitLine(padBetween(left, right, w), w)
}

// renderCoverHeader replaces the library counters with the playback state in
// cover mode, where the number of tracks in the library is irrelevant.
func (m *Model) renderCoverHeader(w int) string {
	left := gradientText(iconLogo+" xngxltuiplayer", colAccent, colAccent2)
	state := strings.ToUpper(m.playbackStateText())
	stateStyle := dimStyle
	if state == "PLAYING" {
		stateStyle = accent2Style
	}
	parts := []string{stateStyle.Render(state)}
	if m.speed != 1 {
		parts = append(parts, accent2Style.Render(audio.SpeedLabel(m.speed)))
	}
	parts = append(parts, dimStyle.Render(fmt.Sprintf("vol %d%%", m.volume)))
	if m.shuffle {
		parts = append(parts, dimStyle.Render("shuffle"))
	}
	right := strings.Join(parts, faintStyle.Render(currentGlyphs.Separator))
	return fitLine(padBetween(left, right, w), w)
}

func (m *Model) playbackStateText() string {
	switch {
	case m.current == nil:
		return "idle"
	case m.ended:
		return "ended"
	case !m.loaded:
		return "loading"
	case m.paused:
		return "paused"
	default:
		return "playing"
	}
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
	allActive := m.view == viewAll
	allSelected := m.focus == focusSidebar && m.sidebarCursor == 0
	lines = append(lines, sidebarRow("All tracks", fmt.Sprintf("%d", len(m.tracks)), allActive, allSelected, false, innerW))

	favCount := len(m.favoriteTracks())
	recentCount := len(m.recentTracks(50))
	lines = append(lines, sidebarRow(iconFavorite+" Favorites", fmt.Sprintf("%d", favCount),
		m.view == viewFavorites, m.focus == focusSidebar && m.sidebarCursor == 1, false, innerW))
	lines = append(lines, sidebarRow("Recent", fmt.Sprintf("%d", recentCount),
		m.view == viewRecent, m.focus == focusSidebar && m.sidebarCursor == 2, false, innerW))
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
			selected := m.focus == focusSidebar && m.sidebarCursor == i+3
			drop := m.dragActive && strings.EqualFold(m.dragTarget, name)
			lines = append(lines, sidebarRow(currentGlyphs.Playlist+" "+cleanText(name), fmt.Sprintf("%d", count), active, selected, drop, innerW))
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
		mark = currentGlyphs.Drop
	} else if active {
		mark = currentGlyphs.Active
	} else if selected {
		mark = currentGlyphs.Cursor
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
			input = ansi.Truncate(input, innerW, ellipsisFor(innerW))
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
		footerRight = "x add" + currentGlyphs.Separator + "D delete"
	} else if m.current != nil && len(m.queue) > 0 {
		footerRight = fmt.Sprintf("queue %d/%d", m.queueIndex+1, len(m.queue))
	} else if len(m.visible) > 0 {
		footerRight = "enter to play"
	}
	lines = append(lines, dimStyle.Render(padBetween("  "+footerLeft, footerRight+"  ", innerW)))

	title := m.viewLabel()
	if m.search != "" {
		title += currentGlyphs.Separator + "/" + truncateText(cleanText(m.search), 24)
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
				"No tracks match " + truncateText("\""+cleanText(m.search)+"\"", maxInt(10, width-16)),
				"Press esc to clear the search.",
			}
		case m.view == viewFavorites:
			message = []string{
				"No favorites yet.",
				"Select a track and press f to add it here.",
			}
		case m.view == viewRecent:
			message = []string{
				"Nothing played yet.",
				"Start a track and it will appear here.",
			}
		case m.view != viewAll:
			message = []string{
				"This playlist is empty.",
				"Open All tracks, select a track and press x.",
			}
		default:
			message = []string{
				"Your library is empty.",
				"Press a to search for a track or paste a link.",
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
			cursor = currentGlyphs.Cursor
		} else if isMarked {
			cursor = currentGlyphs.Checked
		}
		marker := " "
		if isCurrent {
			if m.loaded && !m.paused && !m.ended {
				marker = iconPlay
			} else {
				marker = iconPause
			}
		}
		prefix := cursor + marker

		dur := model.FormatDuration(t.Duration)
		avail := width - 2 - 1 - ansi.StringWidth(dur)
		text := cleanText(t.DisplayTitle())
		if artist := cleanText(t.DisplayArtist()); artist != "" && avail > ansi.StringWidth(text)+4 {
			text += currentGlyphs.Separator + artist
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
		titleLines := wrapText(cleanText(m.current.DisplayTitle()), innerW, 2)
		for _, line := range titleLines {
			lines = append(lines, titleStyle.Render(line))
		}
		lines = append(lines, accent2Style.Render(truncateText(cleanText(m.current.DisplayArtist()), innerW)))
		album := cleanText(m.current.DisplayAlbum())
		if y := strings.TrimSpace(cleanText(m.current.Year)); y != "" {
			album += currentGlyphs.Separator + y
		}
		ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(m.current.Path), "."))
		if ext == "" {
			ext = "AUDIO"
		}
		ext = cleanText(ext)
		if strings.TrimSpace(album) != "" {
			album += currentGlyphs.Separator + ext
		} else {
			album = ext
		}
		lines = append(lines, dimStyle.Render(truncateText(album, innerW)))
		if m.artLoading {
			lines = append(lines, dimStyle.Render("loading cover"+currentGlyphs.Ellipsis))
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
	return renderBox("Now playing "+iconCover+currentGlyphs.Separator+"c", lines, width, height, colFaint, colAccent2)
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

type coverPane struct {
	left   int
	top    int
	width  int
	height int
}

type coverLayoutSpec struct {
	grid   bool
	cover  coverPane
	art    coverPane
	info   coverPane
	upNext coverPane
}

// coverLayout computes the cover screen: a wide left NOW PLAYING panel with
// the cover, track info and transport, and a narrower full-height UP NEXT
// column on the right. It falls back to the compact layout when the panes
// would be too small.
func (m *Model) coverLayout(w, h int) coverLayoutSpec {
	var out coverLayoutSpec
	if w < 88 || h < 23 {
		return out
	}
	const (
		margin = 1
		gap    = 1
	)
	gridW := w - margin*2
	gridH := h - 3
	if gridH < 20 {
		return out
	}

	rightW := maxInt(30, gridW*34/100)
	leftW := gridW - gap - rightW
	if leftW < 48 || rightW < 30 {
		return out
	}

	out.grid = true
	x := (w - gridW) / 2
	y := 1
	out.cover = coverPane{left: x, top: y, width: leftW, height: gridH}
	out.upNext = coverPane{left: x + leftW + gap, top: y, width: rightW, height: gridH}
	out.art, out.info, _ = coverHeroPanes(out.cover)
	return out
}

// coverHeroPanes splits the combined now-playing pane into a left-aligned
// cover, an information block to its right and the rows reserved for the
// spectrum underneath both. When the pane is too narrow for side-by-side
// content the cover is centred and the info pane stays empty.
func coverHeroPanes(pane coverPane) (art, info coverPane, spectrumRows int) {
	innerW := pane.width - 4
	innerH := pane.height - 2
	if innerW < 1 || innerH < 1 {
		return coverPane{}, coverPane{}, 0
	}
	// Split the panel between the hero block and the spectrum. The hero grows
	// with the window but stops at 22 rows so the cover never dwarfs the text
	// block; the rest goes to the spectrum.
	heroRows := minInt(22, maxInt(12, innerH*60/100))
	spectrumRows = innerH - heroRows
	if spectrumRows < 6 {
		spectrumRows = 6
		heroRows = innerH - spectrumRows
	}
	artAreaH := heroRows
	if artAreaH < 4 {
		return coverPane{}, coverPane{}, spectrumRows
	}
	const (
		gap      = 3
		minInfoW = 24
	)
	artH := minInt(artAreaH, (innerW-gap-minInfoW)/2)
	if artH >= 4 {
		artW := artH * 2
		art = coverPane{
			left:   pane.left + 2,
			top:    pane.top + 1 + (artAreaH-artH)/2,
			width:  artW,
			height: artH,
		}
		info = coverPane{
			left:   art.left + artW + gap,
			top:    pane.top + 1,
			width:  innerW - artW - gap,
			height: artAreaH,
		}
		return art, info, spectrumRows
	}

	// Not enough room for the info block: centre the cover as before.
	artH = minInt(artAreaH, innerW/2)
	if artH < 4 {
		return coverPane{}, coverPane{}, spectrumRows
	}
	artW := artH * 2
	art = coverPane{
		left:   pane.left + 2 + (innerW-artW)/2,
		top:    pane.top + 1 + (artAreaH-artH)/2,
		width:  artW,
		height: artH,
	}
	return art, coverPane{}, spectrumRows
}

func coverPaneContains(p coverPane, x, y int) bool {
	return x >= p.left && x < p.left+p.width && y >= p.top && y < p.top+p.height
}

// coverHeroComposition positions the track metadata and the transport inside
// the info column next to the cover. When the column is tall enough the
// metadata is centred in the free space above the transport and the transport
// hugs the bottom of the artwork; shorter panes fall back to a compact
// centred stack. All row offsets are relative to the panel's inner area.
type coverHeroComposition struct {
	art          coverPane
	info         coverPane
	spectrumRows int
	metaLines    []string
	metaTop      int
	progressTop  int
	controlsTop  int
	volumeTop    int
}

func (m *Model) coverHeroComposition(pane coverPane) coverHeroComposition {
	art, info, rows := coverHeroPanes(pane)
	out := coverHeroComposition{art: art, info: info, spectrumRows: rows}
	if art.height <= 0 || info.width <= 0 {
		return out
	}
	artTop := art.top - (pane.top + 1)
	meta := m.coverInfoLines(info.width)
	const transportH = 4
	if len(meta)+1+transportH > art.height {
		meta = dropCoverInfoBlanks(meta)
	}
	if len(meta)+transportH > art.height {
		keep := maxInt(1, art.height-transportH)
		if keep > len(meta) {
			keep = len(meta)
		}
		meta = meta[:keep]
	}
	// Anchor the volume row to the bottom edge of the artwork. The controls and
	// progress sit just above it, with a blank row between controls and volume.
	if art.height >= len(meta)+transportH+1 {
		out.volumeTop = artTop + art.height - 1
		out.controlsTop = out.volumeTop - 2
		out.progressTop = out.controlsTop - 1
		out.metaTop = out.progressTop - len(meta) - 1
		if out.metaTop < artTop {
			out.metaTop = artTop
		}
	} else {
		top := artTop + maxInt(0, (art.height-len(meta)-transportH)/2)
		out.metaTop = top
		out.progressTop = top + len(meta) + 1
		out.controlsTop = out.progressTop + 1
		out.volumeTop = out.controlsTop + 2
	}
	out.metaLines = meta
	return out
}

func dropCoverInfoBlanks(lines []string) []string {
	out := lines[:0]
	for _, line := range lines {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// coverArtSize returns the inner art dimensions and first art row for the
// compact cover layout. The art is wrapped in a one-cell frame, so the frame
// occupies rows artTop-1 through artTop+artH.
func (m *Model) coverArtSize(w, h int) (artW, artH, artTop int) {
	// header + frame + player card + hints + toast
	available := h - 12
	if available < 4 {
		available = 4
	}
	artH = available
	artW = minInt(artH*2, w-6)
	if w >= 96 {
		// Leave room for the UP NEXT panel instead of letting a tall window
		// stretch the art across the full width.
		artW = minInt(artW, w-48)
	}
	artH = artW / 2
	if artH < 4 {
		return 0, 0, 0
	}
	slack := available - artH
	if slack < 0 {
		slack = 0
	}
	return artW, artH, 2 + slack/2
}

// renderCover selects the adaptive cover layout.
func (m *Model) renderCover(w, h int) string {
	layout := m.coverLayout(w, h)
	if layout.grid {
		return m.renderCoverGrid(w, h, layout)
	}
	return m.renderCoverCompact(w, h)
}

// renderCoverCompact draws the fallback single-column cover with the current
// player card. It is used when the window is too narrow or too short for the
// four-pane grid.
func (m *Model) renderCoverCompact(w, h int) string {
	artW, artH, artTop := m.coverArtSize(w, h)
	blank := strings.Repeat(" ", w)
	lines := make([]string, 0, h)
	lines = append(lines, m.renderHeader(w))

	panelShown := false
	if artW > 0 && artH > 0 {
		frameW := artW + 2
		frameLeft := maxInt(0, (w-frameW)/2)
		frameTop := artTop - 1
		for len(lines) < frameTop {
			lines = append(lines, blank)
		}

		panelX := frameLeft + frameW + 2
		panelW := w - panelX - 1
		var panel []string
		if panelW >= 18 && artH+2 >= 6 {
			panel = m.renderCoverPanel(panelW, artH+2)
			panelShown = true
		}

		img := m.artImage
		if img == nil {
			img = placeholderImage(artW, artH*2)
		}
		art := renderArt(img, artW, artH)
		frame := make([]string, 0, artH+2)
		frame = append(frame, borderDimStyle.Render(
			currentGlyphs.BorderTopLeft+strings.Repeat(currentGlyphs.BorderHorizontal, artW)+currentGlyphs.BorderTopRight))
		for i := 0; i < artH; i++ {
			artLine := ""
			if i < len(art) {
				artLine = art[i]
			}
			frame = append(frame, borderDimStyle.Render(currentGlyphs.BorderVertical)+artLine+
				borderDimStyle.Render(currentGlyphs.BorderVertical))
		}
		frame = append(frame, borderDimStyle.Render(
			currentGlyphs.BorderBottomLeft+strings.Repeat(currentGlyphs.BorderHorizontal, artW)+currentGlyphs.BorderBottomRight))

		for i, frameLine := range frame {
			line := strings.Repeat(" ", frameLeft) + frameLine
			if panelShown && i < len(panel) {
				line += strings.Repeat(" ", 2) + panel[i]
			}
			lines = append(lines, fitLine(line, w))
		}
	}

	cardLeft, cardTop, cardW, _ := m.coverCardGeometry(w, h)
	if cardW > 0 {
		for len(lines) < cardTop {
			lines = append(lines, blank)
		}
		card := m.renderCoverPlayerCard(cardW)
		for _, cardLine := range strings.Split(card, "\n") {
			lines = append(lines, fitLine(strings.Repeat(" ", cardLeft)+cardLine, w))
		}
	}

	for len(lines) < h-2 {
		lines = append(lines, blank)
	}
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

func (m *Model) renderCoverGrid(w, h int, layout coverLayoutSpec) string {
	cover := strings.Split(m.renderCoverNowPlayingPane(layout.cover), "\n")
	upNext := strings.Split(m.renderCoverUpNextPane(layout.upNext), "\n")
	blank := strings.Repeat(" ", w)

	left := make([]string, 0, layout.upNext.height)
	left = append(left, cover...)
	leftBlank := strings.Repeat(" ", layout.cover.width)
	rightBlank := strings.Repeat(" ", layout.upNext.width)
	for len(left) < layout.upNext.height {
		left = append(left, leftBlank)
	}
	if len(left) > layout.upNext.height {
		left = left[:layout.upNext.height]
	}
	for len(upNext) < layout.upNext.height {
		upNext = append(upNext, rightBlank)
	}
	if len(upNext) > layout.upNext.height {
		upNext = upNext[:layout.upNext.height]
	}

	body := make([]string, 0, layout.upNext.height)
	gap := layout.upNext.left - (layout.cover.left + layout.cover.width)
	for i := 0; i < layout.upNext.height; i++ {
		body = append(body, fitLine(
			strings.Repeat(" ", layout.cover.left)+left[i]+
				strings.Repeat(" ", maxInt(0, gap))+upNext[i], w))
	}

	lines := []string{m.renderHeader(w)}
	lines = append(lines, body...)
	for len(lines) < h-2 {
		lines = append(lines, blank)
	}
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

func (m *Model) renderCoverNowPlayingPane(pane coverPane) string {
	innerW := pane.width - 4
	innerH := pane.height - 2
	if innerW < 1 || innerH < 1 {
		return renderBox("Now playing", nil, pane.width, pane.height, colFaint, colAccent)
	}
	hero := m.coverHeroComposition(pane)
	artAreaH := innerH - hero.spectrumRows
	if artAreaH < 0 {
		artAreaH = 0
	}
	rows := make([]string, artAreaH)
	art := hero.art

	if art.height > 0 {
		artTop := art.top - (pane.top + 1)
		artLeft := art.left - (pane.left + 2)
		img := m.artImage
		if img == nil {
			img = placeholderImage(art.width, art.height*2)
		}
		for i, artLine := range renderArt(img, art.width, art.height) {
			y := artTop + i
			if y < 0 || y >= len(rows) {
				continue
			}
			rows[y] = strings.Repeat(" ", maxInt(0, artLeft)) + artLine
		}
	} else if artAreaH > 0 {
		message := "No cover"
		if m.current == nil {
			message = "Nothing playing"
		}
		rows[artAreaH/2] = centerLine(dimStyle.Render(message), innerW)
	}

	if hero.info.width > 0 {
		infoLeft := hero.info.left - (pane.left + 2)
		for i, line := range hero.metaLines {
			placeCoverRow(rows, hero.metaTop+i, infoLeft, line)
		}
		placeCoverRow(rows, hero.progressTop, infoLeft,
			m.progressLine(hero.info.width, 0, hero.info.width, ""))
		placeCoverRow(rows, hero.controlsTop, infoLeft,
			m.coverControlsLine(hero.info.width).line)
		placeCoverRow(rows, hero.volumeTop, infoLeft, m.coverVolumeLine(hero.info.width))
	}
	for i := range rows {
		rows[i] = fitLine(rows[i], innerW)
	}

	lines := make([]string, 0, innerH)
	lines = append(lines, rows...)
	if hero.spectrumRows > 0 {
		if innerH >= 16 {
			lines = append(lines, "")
			hero.spectrumRows--
		}
		lines = append(lines, fitLine(sectionStyle.Render("SPECTRUM")+dimStyle.Render("  live FFT"), innerW))
		bars := renderSpectrum(m.spectrum, innerW, maxInt(0, hero.spectrumRows-1))
		lines = append(lines, bars...)
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	return renderBox("Now playing", lines, pane.width, pane.height, colFaint, colAccent)
}

// placeCoverRow writes a styled block at column left in a row that so far only
// contains the artwork (or spaces). Art and info never overlap.
func placeCoverRow(rows []string, y, left int, line string) {
	if y < 0 || y >= len(rows) || line == "" {
		return
	}
	row := rows[y]
	if pad := left - ansi.StringWidth(row); pad > 0 {
		row += strings.Repeat(" ", pad)
	}
	rows[y] = row + line
}

// coverInfoLines renders the large title / artist / metadata block next to the
// cover in the cover-mode hero pane.
func (m *Model) coverInfoLines(width int) []string {
	if width <= 0 {
		return nil
	}
	if m.current == nil {
		return []string{
			titleStyle.Render(truncateText("Nothing playing", width)),
			dimStyle.Render(truncateText("Select a track and press enter", width)),
		}
	}
	lines := make([]string, 0, 7)
	for _, line := range wrapText(cleanText(m.current.DisplayTitle()), width, 2) {
		lines = append(lines, titleStyle.Render(line))
	}
	if artist := strings.TrimSpace(cleanText(m.current.DisplayArtist())); artist != "" {
		lines = append(lines, accent2Style.Render(truncateText(artist, width)))
	}
	if album := cleanText(m.current.DisplayAlbum()); album != "" {
		lines = append(lines, dimStyle.Render(truncateText(album, width)))
	}
	details := make([]string, 0, 2)
	if year := strings.TrimSpace(cleanText(m.current.Year)); year != "" {
		details = append(details, year)
	}
	ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(m.current.Path), "."))
	if ext == "" {
		ext = "AUDIO"
	}
	details = append(details, cleanText(ext))
	if len(details) > 0 {
		lines = append(lines, faintStyle.Render(truncateText(strings.Join(details, currentGlyphs.Separator), width)))
	}
	if m.artLoading {
		lines = append(lines, dimStyle.Render(truncateText("loading cover"+currentGlyphs.Ellipsis, width)))
	}
	return lines
}

func (m *Model) coverUpNextLimit(pane coverPane) int {
	// One row for the position header, one for the footer.
	return minInt(64, maxInt(0, pane.height-4))
}

func (m *Model) renderCoverUpNextPane(pane coverPane) string {
	innerW := pane.width - 4
	innerH := pane.height - 2
	if innerW < 1 || innerH < 1 {
		return renderBox("Up next", nil, pane.width, pane.height, colFaint, colAccent2)
	}
	lines := make([]string, 0, innerH)
	limit := m.coverUpNextLimit(pane)
	tracks := m.upNextTracks(limit)
	position := truncateText(m.coverQueueText(), innerW)
	header := fitLine(faintStyle.Render(position), innerW)
	if mouseHintAvailable() && len(tracks) > 0 && innerW >= 28 {
		header = padBetween(faintStyle.Render(position), faintStyle.Render("click to play"), innerW)
	}
	lines = append(lines, header)

	if len(tracks) == 0 {
		message := "Queue is empty"
		if m.current == nil {
			message = "Nothing playing"
		}
		lines = append(lines, fitLine(dimStyle.Render(message), innerW))
	} else {
		for i, track := range tracks {
			prefix := fmt.Sprintf("%2d. ", i+1)
			label := cleanText(track.DisplayTitle())
			if artist := strings.TrimSpace(cleanText(track.Artist)); artist != "" {
				label = artist + " " + currentGlyphs.Dash + " " + label
			}
			duration := ""
			if track.Duration > 0 {
				duration = model.FormatDuration(track.Duration)
			}
			avail := innerW - ansi.StringWidth(prefix) - ansi.StringWidth(duration)
			if duration != "" {
				avail--
			}
			label = truncateText(label, maxInt(0, avail))
			prefixStyle, labelStyle := dimStyle, textStyle
			if i == 0 {
				prefixStyle, labelStyle = accent2Style, accent2Style.Bold(true)
			}
			left := prefixStyle.Render(prefix) + labelStyle.Render(label)
			row := left
			if duration != "" {
				row = padBetween(left, faintStyle.Render(duration), innerW)
			} else {
				row = fitLine(row, innerW)
			}
			lines = append(lines, row)
		}
	}

	if footer := m.coverUpNextFooter(tracks, innerW); footer != "" {
		for len(lines) < innerH-1 {
			lines = append(lines, "")
		}
		lines = append(lines, footer)
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	return renderBox("Up next", lines, pane.width, pane.height, colFaint, colAccent2)
}

// coverUpNextFooter reports whether the visible list was cut off and how many
// tracks remain, so the panel never looks like it simply stopped.
func (m *Model) coverUpNextFooter(tracks []model.Track, width int) string {
	if len(tracks) == 0 || width <= 0 {
		return ""
	}
	if m.repeat == "one" {
		return fitLine(faintStyle.Render("repeat one"), width)
	}
	all := m.upNextTracks(maxInt(1, len(m.queue)+1))
	hidden := len(all) - len(tracks)
	label := "End of queue"
	if hidden > 0 {
		label = fmt.Sprintf("+%d more", hidden)
	}
	if m.repeat == "all" && width >= 30 {
		return padBetween(faintStyle.Render(label), faintStyle.Render("repeat all"), width)
	}
	return fitLine(faintStyle.Render(label), width)
}

func (m *Model) coverUpNextTrackAt(pane coverPane, x, y int) (model.Track, bool) {
	if x < pane.left+1 || x >= pane.left+pane.width-1 ||
		y < pane.top+1 || y >= pane.top+pane.height-1 {
		return model.Track{}, false
	}
	row := y - (pane.top + 1)
	if row <= 0 {
		return model.Track{}, false
	}
	index := row - 1
	tracks := m.upNextTracks(m.coverUpNextLimit(pane))
	if index < 0 || index >= len(tracks) {
		return model.Track{}, false
	}
	return tracks[index], true
}

func (m *Model) coverCardGeometry(w, h int) (left, top, width, height int) {
	artW, artH, artTop := m.coverArtSize(w, h)
	if artW <= 0 || artH <= 0 {
		return 0, 0, 0, 0
	}
	width = maxInt(artW+2, 44)
	if width > w-4 {
		width = w - 4
	}
	if width < 20 {
		return 0, 0, 0, 0
	}
	left = maxInt(0, (w-width)/2)
	top = artTop + artH + 1
	height = 7
	return
}

func (m *Model) renderCoverPlayerCard(width int) string {
	innerW := maxInt(1, width-4)
	lines := []string{
		m.coverCardTitleLine(innerW),
		m.coverCardMetaLine(innerW),
		m.progressLine(innerW, 0, innerW, ""),
		m.coverControlsLine(innerW).line,
		m.coverVolumeLine(innerW),
	}
	return renderBox("Player", lines, width, 7, colFaint, colAccent)
}

func (m *Model) coverCardTitleLine(width int) string {
	if m.current == nil {
		return centerLine(dimStyle.Render("Nothing playing"), width)
	}
	title := cleanText(m.current.DisplayTitle())
	artist := strings.TrimSpace(cleanText(m.current.Artist))
	queue := m.coverQueueText()
	avail := width - ansi.StringWidth(queue) - 1

	left := ""
	if artist != "" && avail >= 16 {
		artistBudget := minInt(ansi.StringWidth(artist), maxInt(6, avail/3))
		titleBudget := avail - artistBudget - 3
		if titleBudget >= 8 {
			left = titleStyle.Render(truncateText(title, titleBudget)) +
				faintStyle.Render(" "+currentGlyphs.Dash+" ") +
				accent2Style.Render(truncateText(artist, artistBudget))
		}
	}
	if left == "" {
		left = titleStyle.Render(truncateText(title, maxInt(1, avail)))
	}
	return padBetween(left, faintStyle.Render(truncateText(queue, width)), width)
}

func (m *Model) coverCardMetaLine(width int) string {
	meta := dimStyle.Render(truncateText(m.coverMetaText(), width))
	status := strings.ToUpper(m.playbackStateText())
	if m.speed != 1 {
		status += currentGlyphs.Separator + audio.SpeedLabel(m.speed)
	}
	return padBetween(meta, faintStyle.Render(truncateText(status, width)), width)
}

func (m *Model) coverMetaText() string {
	if m.current == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if album := cleanText(m.current.DisplayAlbum()); album != "" {
		parts = append(parts, album)
	}
	if year := strings.TrimSpace(cleanText(m.current.Year)); year != "" {
		parts = append(parts, year)
	}
	ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(m.current.Path), "."))
	if ext == "" {
		ext = "AUDIO"
	}
	parts = append(parts, cleanText(ext))
	return strings.Join(parts, " "+currentGlyphs.Separator+" ")
}

func (m *Model) coverQueueText() string {
	if m.current == nil {
		return ""
	}
	if len(m.queue) == 0 {
		return "queue empty"
	}
	if m.shuffle {
		pos := m.shufflePos
		if pos < 0 || pos >= len(m.shuffleOrder) || len(m.shuffleOrder) != len(m.queue) {
			pos = m.shufflePosition(m.queueIndex)
		}
		if pos >= 0 {
			return fmt.Sprintf("shuffle %d/%d", pos+1, len(m.shuffleOrder))
		}
	}
	pos := m.queueIndex
	if pos < 0 || pos >= len(m.queue) {
		pos = m.queueIndexFor(m.current.Path)
	}
	if pos >= 0 {
		return fmt.Sprintf("queue %d/%d", pos+1, len(m.queue))
	}
	return "queue"
}

type coverControlLayout struct {
	line         string
	shuffleStart int
	shuffleEnd   int
	repeatStart  int
	repeatEnd    int
	prevStart    int
	prevEnd      int
	playStart    int
	playEnd      int
	nextStart    int
	nextEnd      int
}

func (m *Model) coverControlsLine(width int) coverControlLayout {
	out := coverControlLayout{
		shuffleStart: -1,
		shuffleEnd:   -1,
		repeatStart:  -1,
		repeatEnd:    -1,
		prevStart:    -1,
		prevEnd:      -1,
		playStart:    -1,
		playEnd:      -1,
		nextStart:    -1,
		nextEnd:      -1,
	}
	if width <= 0 {
		return out
	}
	gap := 2

	shuffleStyle := dimStyle
	if m.shuffle {
		shuffleStyle = accent2Style.Bold(true)
	}
	repeatStyle := dimStyle
	if m.repeat != "off" {
		repeatStyle = accent2Style.Bold(true)
	}

	left := ""
	leftW := 0
	if iconShuffle != "" {
		out.shuffleStart = 0
		out.shuffleEnd = ansi.StringWidth(iconShuffle)
		left += shuffleStyle.Render(iconShuffle)
		leftW = out.shuffleEnd
	}
	repeatIcon := iconRepeat
	if m.repeat == "one" {
		repeatIcon += " " + iconRepeatOne
	}
	if repeatIcon != "" {
		if leftW > 0 {
			left += strings.Repeat(" ", gap)
			leftW += gap
		}
		out.repeatStart = leftW
		out.repeatEnd = leftW + ansi.StringWidth(repeatIcon)
		left += repeatStyle.Render(repeatIcon)
		leftW = out.repeatEnd
	}

	playIcon := iconPlayButton
	playStyle := accent2Style.Bold(true)
	if m.current == nil {
		playStyle = faintStyle
	}
	if m.current != nil && m.loaded && !m.paused && !m.ended {
		playIcon = iconPauseButton
	}
	prevText := keyStyle.Render(iconPrev)
	playText := playStyle.Render(playIcon)
	nextText := keyStyle.Render(iconNext)
	center := prevText + strings.Repeat(" ", gap) + playText + strings.Repeat(" ", gap) + nextText
	centerW := ansi.StringWidth(center)

	centerStart := maxInt(0, (width-centerW)/2)
	if centerStart < leftW+1 {
		left = ""
		leftW = 0
		out.shuffleStart, out.shuffleEnd = -1, -1
		out.repeatStart, out.repeatEnd = -1, -1
		centerStart = maxInt(0, (width-centerW)/2)
	}

	var b strings.Builder
	if leftW > 0 {
		b.WriteString(left)
		b.WriteString(strings.Repeat(" ", maxInt(0, centerStart-leftW)))
	} else {
		b.WriteString(strings.Repeat(" ", centerStart))
	}
	b.WriteString(center)
	out.line = fitLine(b.String(), width)
	out.prevStart = centerStart
	out.prevEnd = centerStart + ansi.StringWidth(prevText)
	out.playStart = out.prevEnd + gap
	out.playEnd = out.playStart + ansi.StringWidth(playText)
	out.nextStart = out.playEnd + gap
	out.nextEnd = out.nextStart + ansi.StringWidth(nextText)
	return out
}

// coverVolumeLine renders the volume slider as its own row under the
// transport so it can stay wide and mouse-friendly in the hero block.
func (m *Model) coverVolumeLine(width int) string {
	group, _, _ := m.volumeGroup(width)
	return fitLine(group, width)
}

func (m *Model) renderCoverPanel(width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	lines := make([]string, 0, height)
	header := sectionStyle.Render("UP NEXT")
	if len(m.queue) > 0 && m.current != nil {
		pos := m.queueIndex
		label := ""
		if m.shuffle {
			pos = m.shufflePos
			if pos < 0 || pos >= len(m.shuffleOrder) || len(m.shuffleOrder) != len(m.queue) {
				pos = m.shufflePosition(m.queueIndex)
			}
			if pos >= 0 {
				label = fmt.Sprintf("%d/%d", pos+1, len(m.shuffleOrder))
			}
		} else {
			if pos < 0 || pos >= len(m.queue) {
				pos = m.queueIndexFor(m.current.Path)
			}
			if pos >= 0 {
				label = fmt.Sprintf("%d/%d", pos+1, len(m.queue))
			}
		}
		if label != "" {
			header = padBetween(header, faintStyle.Render(label), width)
		}
	}
	lines = append(lines, fitLine(header, width), "")

	limit := minInt(6, maxInt(1, height-3))
	tracks := m.upNextTracks(limit)
	if len(tracks) == 0 {
		message := "Queue is empty"
		if m.current == nil {
			message = "Nothing playing"
		}
		lines = append(lines, fitLine(dimStyle.Render(message), width))
	} else {
		for i, track := range tracks {
			prefix := fmt.Sprintf("%d. ", i+1)
			label := cleanText(track.DisplayTitle())
			if artist := strings.TrimSpace(cleanText(track.Artist)); artist != "" {
				label = artist + " " + currentGlyphs.Dash + " " + label
			}
			avail := width - ansi.StringWidth(prefix)
			lines = append(lines, fitLine(dimStyle.Render(prefix)+textStyle.Render(truncateText(label, maxInt(0, avail))), width))
		}
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

// upNextTracks returns the tracks that will actually follow the current one.
// With shuffle enabled it follows the precomputed shuffle order, so the list
// matches playback instead of showing the raw queue.
func (m *Model) upNextTracks(limit int) []model.Track {
	if limit <= 0 || len(m.queue) == 0 || m.current == nil {
		return nil
	}
	if m.repeat == "one" {
		return []model.Track{*m.current}
	}
	byPath := make(map[string]model.Track, len(m.tracks))
	for _, track := range m.tracks {
		byPath[trackKey(track.Path)] = track
	}
	out := make([]model.Track, 0, limit)

	if m.shuffle {
		pos := m.ensureShuffleOrder()
		if pos < 0 {
			return nil
		}
		for offset := 1; offset < len(m.shuffleOrder) && len(out) < limit; offset++ {
			orderIndex := pos + offset
			if orderIndex >= len(m.shuffleOrder) {
				if m.repeat != "all" {
					break
				}
				orderIndex %= len(m.shuffleOrder)
				if orderIndex == pos {
					break
				}
			}
			queueIndex := m.shuffleOrder[orderIndex]
			if queueIndex < 0 || queueIndex >= len(m.queue) {
				continue
			}
			track, ok := byPath[trackKey(m.queue[queueIndex])]
			if !ok {
				continue
			}
			out = append(out, track)
		}
		return out
	}

	start := m.queueIndex
	if start < 0 || start >= len(m.queue) || trackKey(m.queue[start]) != trackKey(m.current.Path) {
		start = m.queueIndexFor(m.current.Path)
	}
	if start < 0 {
		return nil
	}
	for offset := 1; offset < len(m.queue) && len(out) < limit; offset++ {
		idx := start + offset
		if idx >= len(m.queue) {
			if m.repeat != "all" {
				break
			}
			idx %= len(m.queue)
			if idx == start {
				break
			}
		}
		track, ok := byPath[trackKey(m.queue[idx])]
		if !ok {
			continue
		}
		out = append(out, track)
	}
	return out
}

func (m *Model) renderCoverHints() string {
	w, _ := m.dims()
	if w >= 110 {
		return hintLine(
			keyHint("c / esc", "close"),
			keyHint("space", "pause"),
			keyHint(currentGlyphs.LeftRight, "seek"),
			keyHint("-/+", "volume"),
			keyHint("[ ]", "speed"),
			keyHint("Q", "EQ"),
			keyHint("t", "theme"),
			keyHint("?", "help"),
			keyHint("q", "quit"),
		)
	}
	if w >= 80 {
		return hintLine(
			keyHint("c / esc", "close"),
			keyHint("space", "pause"),
			keyHint(currentGlyphs.LeftRight, "seek"),
			keyHint("Q", "EQ"),
			keyHint("t", "theme"),
			keyHint("?", "help"),
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
		m.playerArtistLine(innerW),
		m.playerProgressLine(innerW),
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
	return m.playerTitleLayout(innerW).line
}

type playerTitleLayout struct {
	line string

	shuffleStart, shuffleEnd int
	prevStart, prevEnd       int
	playStart, playEnd       int
	nextStart, nextEnd       int
	repeatStart, repeatEnd   int

	volBarStart, volBarEnd int
	volMinus, volPlus      int
}

func (m *Model) playerTitleLayout(innerW int) playerTitleLayout {
	var out playerTitleLayout
	out.volBarStart, out.volBarEnd = -1, -1
	out.volMinus, out.volPlus = -1, -1
	if innerW <= 0 {
		return out
	}

	shuffleIcon := iconShuffle
	shuffleStyle := dimStyle
	if m.shuffle {
		shuffleStyle = accent2Style.Bold(true)
	}
	repeatIcon := iconRepeat
	if m.repeat == "one" {
		repeatIcon = iconRepeat + " " + iconRepeatOne
	}
	repeatStyle := dimStyle
	if m.repeat != "off" {
		repeatStyle = accent2Style.Bold(true)
	}
	playing := m.current != nil && m.loaded && !m.paused && !m.ended
	playStyle := accent2Style.Bold(true)
	if m.current == nil {
		playStyle = faintStyle
	}
	// The play/pause glyph is a single text cell with no colored background.
	// Center the glyph itself; the repeat/shuffle controls can make the
	// surrounding group asymmetric.
	playIcon := iconPlayButton
	if playing {
		playIcon = iconPauseButton
	}
	playText := playStyle.Render(playIcon)

	prevIcon := iconPrev
	nextIcon := iconNext
	gap := 2
	shuffleW := ansi.StringWidth(shuffleIcon)
	prevW := ansi.StringWidth(prevIcon)
	playW := ansi.StringWidth(playText)
	nextW := ansi.StringWidth(nextIcon)
	// Reserve space for the gap and the "one" indicator in every mode so
	// cycling repeat does not shift the transport.
	repeatW := ansi.StringWidth(iconRepeat) + 1 + ansi.StringWidth(iconRepeatOne)
	transportW := prevW + gap + playW + gap + nextW
	if shuffleW > 0 {
		transportW += shuffleW + gap
	}
	if repeatW > 0 {
		transportW += gap + repeatW
	}

	// At extremely narrow widths keep the core transport usable.
	if transportW > innerW-2 {
		shuffleIcon, repeatIcon = "", ""
		shuffleW, repeatW = 0, 0
		transportW = prevW + gap + playW + gap + nextW
	}

	playStart := (innerW - playW) / 2
	if playStart < 0 {
		playStart = 0
	}
	playEnd := playStart + playW
	out.playStart = playStart
	out.playEnd = playEnd
	out.prevEnd = playStart - gap
	out.prevStart = out.prevEnd - prevW
	out.shuffleEnd = out.prevStart - gap
	out.shuffleStart = out.shuffleEnd - shuffleW
	out.nextStart = playEnd + gap
	out.nextEnd = out.nextStart + nextW
	out.repeatStart = out.nextEnd + gap
	out.repeatEnd = out.repeatStart + repeatW

	groupStart := out.shuffleStart
	if shuffleW == 0 {
		groupStart = out.prevStart
	}
	groupEnd := out.repeatEnd
	if repeatW == 0 {
		groupEnd = out.nextEnd
	}
	leftW := groupStart - 1
	if leftW < 0 {
		leftW = 0
	}
	rightStart := groupEnd + 1
	rightW := innerW - rightStart
	if rightW < 0 {
		rightW = 0
	}

	repeatText := ""
	if repeatW > 0 {
		repeatText = centerPad(repeatIcon, repeatW)
	}
	transport := ""
	if shuffleW > 0 {
		transport += shuffleStyle.Render(shuffleIcon) + strings.Repeat(" ", gap)
	}
	transport += keyStyle.Render(prevIcon) + strings.Repeat(" ", gap) +
		playText + strings.Repeat(" ", gap) + keyStyle.Render(nextIcon)
	if repeatW > 0 {
		transport += strings.Repeat(" ", gap) + repeatStyle.Render(repeatText)
	}

	volStr, barStart, barEnd := m.volumeGroup(rightW)
	volW := ansi.StringWidth(volStr)
	if volW > rightW {
		volStr = ansi.Truncate(volStr, rightW, "")
		volW = ansi.StringWidth(volStr)
	}
	volGroupStart := innerW - volW
	rightPart := strings.Repeat(" ", maxInt(0, rightW-volW)) + volStr
	if barStart >= 0 {
		out.volBarStart = volGroupStart + barStart
		out.volBarEnd = volGroupStart + barEnd
		out.volMinus = out.volBarStart - 1
		out.volPlus = out.volBarEnd
	}

	line := fitLineEllipsis(m.playerNowText(), leftW) + " " + transport + " " + rightPart
	out.line = fitLine(line, innerW)
	return out
}

func (m *Model) playerNowText() string {
	icon := currentGlyphs.Stop
	iconStyle := faintStyle
	text := dimStyle.Render("Nothing playing")
	if m.current != nil {
		if m.loaded && !m.paused && !m.ended {
			icon = iconPlay
		} else if m.paused || m.loaded {
			icon = iconPause
		}
		iconStyle = accent2Style
		text = titleStyle.Render(cleanText(m.current.DisplayTitle()))
	}
	speed := ""
	if m.speed != 1 {
		speed = accent2Style.Render(audio.SpeedLabel(m.speed)) + " "
	}
	return iconStyle.Render(icon) + " " + speed + text
}

func (m *Model) playerArtistLine(innerW int) string {
	if m.current == nil {
		return fitLineEllipsis("  "+dimStyle.Render("select a track and press enter"), innerW)
	}
	return fitLineEllipsis("  "+dimStyle.Render(cleanText(m.current.DisplayArtist())), innerW)
}

// volumeGroup returns the right-aligned volume widget and the bar offsets
// relative to the widget start. Negative offsets mean the compact widget has
// no interactive slider.
func (m *Model) volumeGroup(width int) (string, int, int) {
	if width < 4 {
		return "", -1, -1
	}
	if width >= 17 {
		barW := width - 11
		if barW > 30 {
			barW = 30
		}
		if barW >= 6 {
			volColor := colGreen
			if m.volume == 0 {
				volColor = colRed
			}
			bar := lipgloss.NewStyle().Foreground(volColor).Render(volumeBar(m.volume, barW))
			group := dimStyle.Render("vol ") + keyStyle.Render("-") + bar + keyStyle.Render("+") +
				dimStyle.Render(fmt.Sprintf(" %3d%%", m.volume))
			return group, 5, 5 + barW
		}
	}
	if width >= 8 {
		return dimStyle.Render("vol ") + textStyle.Render(fmt.Sprintf("%3d%%", m.volume)), -1, -1
	}
	pct := fmt.Sprintf("%d%%", m.volume)
	if width >= ansi.StringWidth(pct) {
		return textStyle.Render(pct), -1, -1
	}
	return "", -1, -1
}

func (m *Model) playerProgressLine(innerW int) string {
	start, segW := m.playerCenterSegment(innerW)
	return m.progressLine(innerW, start, segW, m.playerProgressRight(innerW, start+segW))
}

func (m *Model) playerProgressRight(innerW, segEnd int) string {
	space := innerW - segEnd
	if space < 6 {
		return ""
	}
	queue := ""
	if m.current != nil && len(m.queue) > 0 {
		queue = fmt.Sprintf("%d/%d", m.queueIndex+1, len(m.queue))
	}
	qW := ansi.StringWidth(queue)
	next := m.peekNextTitle()
	if next != "" {
		avail := space - 8 // "next  " plus two cells of breathing room
		if queue != "" {
			avail -= qW + 2
		}
		if avail >= 4 {
			right := dimStyle.Render("next  ") + textStyle.Render(ansi.Truncate(next, avail, ellipsisFor(avail)))
			if queue != "" {
				right += faintStyle.Render("  ") + dimStyle.Render(queue)
			}
			return right
		}
	}
	if queue != "" && space >= qW+1 {
		return dimStyle.Render(queue)
	}
	return ""
}

// coverProgressLine is used by the full-screen cover, where there is no player
// box and the bar can use the whole terminal width.
func (m *Model) progressLine(innerW, start, segW int, rightText string) string {
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
	var segment string
	if segW < timeW*2+3 {
		barW := maxInt(1, segW)
		segment = progressBar(progress, barW, m.current != nil && dur > 0)
	} else {
		barW := segW - timeW*2 - 2
		if barW < 1 {
			barW = 1
		}
		bar := progressBar(progress, barW, m.current != nil && dur > 0)
		left := dimStyle.Render(fmt.Sprintf("%-*s", timeW, leftLabel))
		right := dimStyle.Render(fmt.Sprintf("%*s", timeW, rightLabel))
		segment = left + " " + bar + " " + right
	}
	if ansi.StringWidth(segment) > segW {
		segment = truncateText(segment, segW)
	}
	if pad := segW - ansi.StringWidth(segment); pad > 0 {
		segment += strings.Repeat(" ", pad)
	}

	rightW := ansi.StringWidth(rightText)
	if rightText != "" && innerW-(start+segW) >= rightW+1 {
		gap := innerW - rightW - (start + segW)
		return strings.Repeat(" ", maxInt(0, start)) + segment + strings.Repeat(" ", gap) + rightText
	}
	return fitLine(strings.Repeat(" ", maxInt(0, start))+segment, innerW)
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
		return cleanText(t.DisplayTitle())
	}
	return ""
}

func progressBar(progress float64, width int, enabled bool) string {
	if width <= 0 {
		return ""
	}
	if !enabled {
		return faintStyle.Render(strings.Repeat(currentGlyphs.ProgressEmpty, width))
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
		b.WriteString(accent2Style.Render(strings.Repeat(currentGlyphs.ProgressFull, filled)))
	}
	if filled < width {
		b.WriteString(faintStyle.Render(strings.Repeat(currentGlyphs.ProgressEmpty, width-filled)))
	}
	return b.String()
}

// playerContentX is the terminal column where the player box inner content
// starts (left border + one cell of padding).
const playerContentX = 2

// volumeBarBounds returns the terminal x range of the volume bar in the player
// title line. A negative range means the compact volume widget has no slider.
func (m *Model) volumeBarBounds(width int) (start, end int) {
	innerW := maxInt(1, width-4)
	layout := m.playerTitleLayout(innerW)
	if layout.volBarStart < 0 {
		return -1, -1
	}
	return playerContentX + layout.volBarStart, playerContentX + layout.volBarEnd
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
	layout := m.coverLayout(width, m.height)
	if layout.grid {
		hero := m.coverHeroComposition(layout.cover)
		innerW := maxInt(1, hero.info.width)
		contentX := hero.info.left
		if hero.info.width <= 0 {
			return 0, maxInt(1, width)
		}
		rightLabel := model.FormatDuration(m.duration)
		timeW := maxInt(7, ansi.StringWidth(rightLabel))
		if innerW < timeW*2+3 {
			return contentX, maxInt(1, innerW)
		}
		barWidth = innerW - timeW*2 - 2
		if barWidth < 1 {
			barWidth = 1
		}
		return contentX + timeW + 1, barWidth
	}
	cardLeft, _, cardW, _ := m.coverCardGeometry(width, m.height)
	if cardW <= 0 {
		return 0, maxInt(1, width)
	}
	innerW := maxInt(1, cardW-4)
	contentX := cardLeft + 2
	rightLabel := model.FormatDuration(m.duration)
	timeW := maxInt(7, ansi.StringWidth(rightLabel))
	if innerW < timeW*2+3 {
		return contentX, maxInt(1, innerW)
	}
	barWidth = innerW - timeW*2 - 2
	if barWidth < 1 {
		barWidth = 1
	}
	return contentX + timeW + 1, barWidth
}

// coverVolumeBarBounds returns the terminal x range of the volume slider in
// cover mode. A negative range means the current layout is too narrow for a
// slider and only the percentage is shown.
func (m *Model) coverVolumeBarBounds(width int) (start, end int) {
	innerW := 0
	contentX := 0
	layout := m.coverLayout(width, m.height)
	if layout.grid {
		hero := m.coverHeroComposition(layout.cover)
		if hero.info.width <= 0 {
			return -1, -1
		}
		innerW = hero.info.width
		contentX = hero.info.left
	} else {
		cardLeft, _, cardW, _ := m.coverCardGeometry(width, m.height)
		if cardW <= 0 {
			return -1, -1
		}
		innerW = cardW - 4
		contentX = cardLeft + 2
	}
	if innerW < 17 {
		return -1, -1
	}
	barW := innerW - 11
	if barW > 30 {
		barW = 30
	}
	if barW < 6 {
		return -1, -1
	}
	start = contentX + 5
	return start, start + barW
}

const volumeSteps = 7

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
	partial := (units % 100) * (volumeSteps + 1) / 100
	var b strings.Builder
	b.WriteString(strings.Repeat(currentGlyphs.VolumeFull, full))
	empty := width - full
	if empty > 0 && partial > 0 && len(currentGlyphs.VolumePartial) > 0 {
		idx := partial - 1
		if idx >= len(currentGlyphs.VolumePartial) {
			idx = len(currentGlyphs.VolumePartial) - 1
		}
		b.WriteRune(currentGlyphs.VolumePartial[idx])
		empty--
	}
	b.WriteString(strings.Repeat(currentGlyphs.VolumeEmpty, empty))
	return b.String()
}

func (m *Model) renderHints() string {
	w, _ := m.dims()
	if m.searching {
		return hintLine(
			keyHint("type", "filter"),
			keyHint(currentGlyphs.UpDown, "select"),
			keyHint("enter", "play"),
			keyHint("esc", "clear"),
		)
	}
	if m.selectionCount() > 0 {
		if w >= 96 && mouseHintAvailable() {
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
				keyHint(currentGlyphs.UpDown, "playlists"),
				keyHint("enter", "open"),
				keyHint("tab", "tracks"),
				keyHint("P", "new"),
				keyHint("R", "rename"),
				keyHint("D", "delete"),
				keyHint("a", "search / add"),
				keyHint("?", "help"),
				keyHint("q", "quit"),
			)
		}
		if w >= 72 {
			return hintLine(
				keyHint(currentGlyphs.UpDown, "move"),
				keyHint("enter", "open"),
				keyHint("tab", "tracks"),
				keyHint("P", "new"),
				keyHint("?", "help"),
				keyHint("q", "quit"),
			)
		}
		return hintLine(
			keyHint(currentGlyphs.UpDown, "move"),
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
			keyHint("a", "search / add"),
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
		if m.startupNote != "" && time.Now().Before(m.startupUntil) {
			return renderToast(m.startupNote, toastInfo, w)
		}
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
	case modalAddLink:
		if len(m.linkResults) > 0 {
			return hintLine(
				keyHint("space", "preview"),
				keyHint("ctrl+space / x", "select"),
				keyHint("enter", "download"),
				keyHint("tab", "source"),
				keyHint("esc", "back"),
			)
		}
		if m.linkLoading {
			return hintLine(
				keyHint("tab", "source"),
				keyHint("esc", "cancel"),
			)
		}
		return hintLine(
			keyHint("enter", "search / confirm"),
			keyHint("tab", "source"),
			keyHint("ctrl+v", "paste"),
			keyHint("esc", "close"),
		)
	case modalNewPlaylist:
		return hintLine(
			keyHint("enter", "confirm"),
			keyHint("esc", "cancel"),
			keyHint("ctrl+v", "paste"),
		)
	case modalPreview:
		return hintLine(
			keyHint("enter", "queue all"),
			keyHint("esc", "cancel"),
		)
	case modalEQ:
		return hintLine(
			keyHint(currentGlyphs.LeftRight, "band"),
			keyHint(currentGlyphs.UpDown, "gain"),
			keyHint("0", "reset"),
			keyHint("1-4", "presets"),
			keyHint("-/+", "preamp"),
			keyHint("esc", "close"),
		)
	case modalImportM3U, modalExportM3U:
		return hintLine(
			keyHint("enter", "confirm"),
			keyHint("esc", "cancel"),
			keyHint("ctrl+v", "paste"),
		)
	case modalAddToPlaylist:
		return hintLine(
			keyHint(currentGlyphs.UpDown, "choose"),
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
				keyHint(currentGlyphs.UpDown, "select"),
				keyHint("r", "retry"),
				keyHint("c", "cancel"),
				keyHint("C", "clear"),
				keyHint("esc", "close"),
			)
		}
		return hintLine(
			keyHint(currentGlyphs.UpDown, "select"),
			keyHint("r/R", "retry"),
			keyHint("c", "cancel"),
			keyHint("C", "clear finished"),
			keyHint("a", "search / add"),
			keyHint("esc", "close"),
		)
	case modalHelp:
		return hintLine(
			keyHint(currentGlyphs.UpDown, "scroll"),
			keyHint("esc", "close"),
			keyHint("?", "close"),
		)
	case modalEditTags:
		return hintLine(
			keyHint("ctrl+s", "save"),
			keyHint("tab", "next field"),
			keyHint("esc", "cancel"),
		)
	case modalDuplicates:
		return hintLine(
			keyHint(currentGlyphs.UpDown, "keeper"),
			keyHint("n / p", "group"),
			keyHint("enter", "merge"),
			keyHint("esc", "close"),
		)
	case modalThemes:
		return hintLine(
			keyHint(currentGlyphs.UpDown, "preview"),
			keyHint("enter", "apply"),
			keyHint("esc", "cancel"),
		)
	case modalUpdate:
		switch m.updatePhase {
		case "", "error":
			return hintLine(
				keyHint("enter", "update"),
				keyHint("esc", "later"),
			)
		default:
			return dimStyle.Render("Please wait: the player will restart after the update.")
		}
	}
	return ""
}

func (m *Model) renderLoading(w, h int) string {
	frames := currentGlyphs.Spinner
	if len(frames) == 0 {
		frames = []rune("|/-\\")
	}
	frame := frames[m.spinner%len(frames)]

	logo := gradientText(iconLogo+" xngxltuiplayer", colAccent, colAccent2)
	var body string
	if m.loadErr != nil {
		body = logo + "\n\n" +
			errorStyle.Render("Could not load the library") + "\n\n" +
			dimStyle.Render(truncateText(cleanText(m.loadErr.Error()), maxInt(20, w-12))) + "\n\n" +
			hintLine(keyHint("r", "retry"), keyHint("q", "quit"))
	} else {
		body = logo + "\n\n" +
			accent2Style.Render(string(frame)) + "  " + textStyle.Render("Scanning your library"+currentGlyphs.Ellipsis) + "\n\n" +
			dimStyle.Render(truncateText(cleanText(m.cfg.MusicDir), maxInt(20, w-12))) + "\n" +
			dimStyle.Render("The first scan may take a moment.")
	}
	return placeBlock(body, w, h)
}

func (m *Model) renderTooSmall(w, h int) string {
	body := gradientText(iconLogo+" xngxltuiplayer", colAccent, colAccent2) + "\n\n" +
		textStyle.Render("Terminal too small") + "\n" +
		dimStyle.Render(fmt.Sprintf("%dx%d - need at least 54x16", w, h))
	return placeBlock(body, w, h)
}

// fitLineEllipsis pads a line to width and uses an ellipsis when it has to cut
// styled text.
func centerPad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(s)
	if w >= width {
		return ansi.Truncate(s, width, "")
	}
	left := (width - w) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-w-left)
}

func fitLineEllipsis(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, ellipsisFor(width))
	}
	if gap := width - ansi.StringWidth(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
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

// sanitizeFrame is the last line of defence before Bubble Tea writes to the
// terminal. It keeps ANSI styles, converts invalid UTF-8, replaces tabs and
// other control characters, clips every line to the terminal width and makes
// sure the frame has exactly the expected number of rows. A broken badge or
// error message can therefore never wrap the whole UI.
func sanitizeFrame(view string, width, height int) string {
	if width <= 0 || height <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	out := make([]string, height)
	for i := range out {
		line := ""
		if i < len(lines) {
			line = sanitizeANSIText(lines[i])
		}
		line = ansi.Truncate(line, width, "")
		if gap := width - ansi.StringWidth(line); gap > 0 {
			line += strings.Repeat(" ", gap)
		}
		out[i] = line
	}
	return strings.Join(out, "\n")
}

// sanitizeANSIText removes control characters while preserving CSI/OSC escape
// sequences used by lipgloss for colours and backgrounds.
func sanitizeANSIText(s string) string {
	s = strings.ToValidUTF8(s, "?")
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c == 0x1b {
			start := i
			i++
			switch {
			case i < len(s) && s[i] == '[':
				i++
				for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3f {
					i++
				}
				if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7e {
					i++
				}
			case i < len(s) && s[i] == ']':
				i++
				for i < len(s) {
					if s[i] == 0x07 {
						i++
						break
					}
					if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			case i < len(s):
				i++
			}
			b.WriteString(s[start:i])
			continue
		}
		if c < 0x20 || c == 0x7f {
			if c == '\t' || c == '\r' {
				b.WriteByte(' ')
			}
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}
