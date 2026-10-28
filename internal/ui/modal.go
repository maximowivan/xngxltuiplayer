package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"tuiplay/internal/download"
	"tuiplay/internal/model"
)

type keyBinding struct {
	key  string
	desc string
}

// renderModal returns a ready-to-place box for the active modal. contentH is
// the available height of the content region.
func (m *Model) renderModal(w, contentH int) string {
	switch m.modal {
	case modalAddLink:
		return m.renderAddLinkModal(w, contentH)
	case modalNewPlaylist:
		return m.renderNewPlaylistModal(w, contentH)
	case modalAddToPlaylist:
		return m.renderAddToPlaylistModal(w, contentH)
	case modalConfirmRemoveFromPlaylist:
		return m.renderRemoveFromPlaylistModal(w, contentH)
	case modalConfirmDeleteTrack:
		return m.renderDeleteTrackModal(w, contentH)
	case modalConfirmDeletePlaylist:
		return m.renderDeletePlaylistModal(w, contentH)
	case modalHelp:
		return m.renderHelpModal(w, contentH)
	case modalDownloads:
		return m.renderDownloadsModal(w, contentH)
	case modalImportM3U, modalExportM3U:
		return m.renderM3UModal(w, contentH)
	case modalEditTags:
		return m.renderTagModal(w, contentH)
	case modalDuplicates:
		return m.renderDuplicatesModal(w, contentH)
	case modalThemes:
		return m.renderThemePicker(w, contentH)
	case modalUpdate:
		return m.renderUpdateModal(w, contentH)
	}
	return ""
}

func modalWidth(w int) int {
	return minInt(76, maxInt(36, w-8))
}

func modalHeight(contentH, desired int) int {
	return minInt(maxInt(8, desired), maxInt(8, contentH-2))
}

func (m *Model) renderAddLinkModal(w, contentH int) string {
	width := modalWidth(w)
	innerW := width - 4
	height := modalHeight(contentH, 10)

	lines := []string{
		textStyle.Render(truncateText("Paste a link and xngxltuiplayer will download, convert and tag it.", innerW)),
		"",
		accent2Style.Render("Spotify track") + faintStyle.Render(currentGlyphs.Separator) +
			accent2Style.Render("SoundCloud track") + faintStyle.Render(currentGlyphs.Separator) +
			accent2Style.Render("YouTube") + faintStyle.Render(currentGlyphs.Separator) +
			accent2Style.Render("audio URL"),
		"",
		fitLine(m.linkInput.View(), innerW),
		"",
	}
	if m.modalErr != "" {
		lines = append(lines, errorStyle.Render(truncateText(cleanText(m.modalErr), innerW)))
	} else {
		lines = append(lines, dimStyle.Render("The file is saved to your music folder as MP3."))
	}
	lines = append(lines, dimStyle.Render("Spotify playlists and albums are not supported "+currentGlyphs.Dash+" use single tracks."))
	return renderBox("Add music", lines, width, height, colAccent, colAccent)
}

func (m *Model) renderM3UModal(w, contentH int) string {
	width := modalWidth(w)
	innerW := width - 4
	height := modalHeight(contentH, 11)
	importing := m.modal == modalImportM3U

	title := "Export M3U"
	description := fmt.Sprintf("Write the current list (%d tracks) to an M3U8 file.", len(m.visible))
	if importing {
		title = "Import M3U"
		description = "Read an M3U/M3U8 playlist and create a playlist from it."
	}

	lines := []string{
		textStyle.Render(truncateText(description, innerW)),
		"",
		fitLine(m.m3uInput.View(), innerW),
		"",
	}
	if m.modalErr != "" {
		lines = append(lines, errorStyle.Render(truncateText(cleanText(m.modalErr), innerW)))
	} else {
		lines = append(lines, dimStyle.Render("Relative paths are resolved against the music folder."))
	}
	return renderBox(title, lines, width, height, colAccent, colAccent)
}

func (m *Model) renderNewPlaylistModal(w, contentH int) string {
	width := modalWidth(w)
	innerW := width - 4
	height := modalHeight(contentH, 9)

	lines := []string{
		textStyle.Render(truncateText("Give the new playlist a name. You can add tracks with x.", innerW)),
		"",
		fitLine(m.nameInput.View(), innerW),
		"",
	}
	if m.modalErr != "" {
		lines = append(lines, errorStyle.Render(truncateText(cleanText(m.modalErr), innerW)))
	} else {
		lines = append(lines, dimStyle.Render("Up to 48 characters. Names are case-insensitive."))
	}
	return renderBox("New playlist", lines, width, height, colAccent, colAccent)
}

func (m *Model) renderAddToPlaylistModal(w, contentH int) string {
	width := modalWidth(w)
	innerW := width - 4
	items := m.playlists.List()
	height := modalHeight(contentH, minInt(18, 8+len(items)))
	innerH := height - 2

	rows := maxInt(1, innerH-4)
	if m.modalCursor < 0 {
		m.modalCursor = 0
	}
	if m.modalCursor >= len(items) {
		m.modalCursor = maxInt(0, len(items)-1)
	}
	offset := 0
	if m.modalCursor >= rows {
		offset = m.modalCursor - rows + 1
	}

	header := dimStyle.Render("Track  ") + titleStyle.Render(truncateText(cleanText(m.pendingTrack.DisplayTitle()), maxInt(8, innerW-7)))
	if len(m.pendingPaths) > 1 {
		header = dimStyle.Render("Tracks  ") + titleStyle.Render(fmt.Sprintf("%d selected", len(m.pendingPaths)))
	}
	lines := []string{header, ""}
	if len(items) == 0 {
		lines = append(lines, dimStyle.Render("No playlists yet. Press P to create one."))
	}
	end := minInt(len(items), offset+rows)
	for i := offset; i < end; i++ {
		pl := items[i]
		cursor := " "
		if i == m.modalCursor {
			cursor = currentGlyphs.Cursor
		}
		contained := 0
		for _, path := range m.pendingPaths {
			if m.playlists.Contains(pl.Name, path) {
				contained++
			}
		}
		mark := " "
		switch {
		case len(m.pendingPaths) > 0 && contained == len(m.pendingPaths):
			mark = currentGlyphs.Check
		case contained > 0:
			mark = currentGlyphs.Partial
		}
		left := cursor + " " + mark + " " + cleanText(pl.Name)
		right := fmt.Sprintf("%d", len(pl.Tracks))
		if len(m.pendingPaths) > 0 && contained == len(m.pendingPaths) {
			right = currentGlyphs.Check + " " + right
		} else if contained > 0 {
			right = fmt.Sprintf("%d/%d", contained, len(m.pendingPaths))
		}
		row := padBetween(left, right, innerW)
		switch {
		case i == m.modalCursor:
			lines = append(lines, selectedStyle.Render(row))
		case len(m.pendingPaths) > 0 && contained == len(m.pendingPaths):
			lines = append(lines, dimStyle.Render(row))
		default:
			lines = append(lines, textStyle.Render(row))
		}
	}
	for len(lines) < innerH-1 {
		lines = append(lines, "")
	}
	lines = append(lines, dimStyle.Render(currentGlyphs.Check+" already there"+currentGlyphs.Separator+currentGlyphs.Partial+" partly"+currentGlyphs.Separator+"Enter adds"))
	return renderBox("Add to playlist", lines, width, height, colAccent, colAccent)
}

func (m *Model) renderRemoveFromPlaylistModal(w, contentH int) string {
	width := modalWidth(w)
	innerW := width - 4
	count := len(m.deleteTracks)
	height := modalHeight(contentH, 10+minInt(maxInt(count, 1), 5))

	playlist := cleanText(m.view)
	if playlist == "" {
		playlist = "this playlist"
	}
	noun := "track"
	if count > 1 {
		noun = "tracks"
	}
	lines := []string{
		warningStyle.Render(fmt.Sprintf("Remove %d %s from \"%s\"?", count, noun, truncateText(playlist, maxInt(8, innerW-20)))),
		"",
	}
	shown := minInt(count, 5)
	for i := 0; i < shown; i++ {
		lines = append(lines, dimStyle.Render(currentGlyphs.Bullet+" ")+textStyle.Render(truncateText(cleanText(m.deleteTracks[i].DisplayTitle()), maxInt(8, innerW-2))))
	}
	if count > shown {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  %sand %d more", currentGlyphs.Ellipsis, count-shown)))
	}
	lines = append(lines,
		"",
		textStyle.Render("Audio files stay on disk and remain in All tracks."),
		"",
		dimStyle.Render("y / enter remove"+currentGlyphs.Separator+"d delete from disk"+currentGlyphs.Separator+"esc cancel"),
	)
	return renderBox("Remove from playlist", lines, width, height, colYellow, colYellow)
}

func (m *Model) renderDeleteTrackModal(w, contentH int) string {
	width := modalWidth(w)
	innerW := width - 4
	count := len(m.deleteTracks)
	height := modalHeight(contentH, 11)
	if count > 1 {
		height = modalHeight(contentH, 10+minInt(count, 5))
	}

	var lines []string
	if count <= 1 {
		t := model.Track{}
		if count == 1 {
			t = m.deleteTracks[0]
		}
		title := cleanText(t.DisplayTitle())
		if title == "" {
			title = "this track"
		}
		lines = []string{
			warningStyle.Render(iconDelete + " This permanently deletes the audio file from disk."),
			"",
			titleStyle.Render(truncateText(title, innerW)),
			accent2Style.Render(truncateText(cleanText(t.DisplayArtist()), innerW)),
			dimStyle.Render("file  ") + textStyle.Render(truncateText(cleanText(filepath.Base(t.Path)), maxInt(8, innerW-6))),
			"",
			textStyle.Render("It will also be removed from every playlist."),
			"",
			dimStyle.Render("Press y or enter to delete" + currentGlyphs.Separator + "esc to cancel."),
		}
	} else {
		lines = []string{
			warningStyle.Render(fmt.Sprintf(iconDelete+" This permanently deletes %d audio files from disk.", count)),
			"",
			titleStyle.Render(fmt.Sprintf("%d tracks", count)),
		}
		shown := minInt(count, 5)
		for i := 0; i < shown; i++ {
			t := m.deleteTracks[i]
			lines = append(lines, dimStyle.Render(currentGlyphs.Bullet+" ")+textStyle.Render(truncateText(cleanText(t.DisplayTitle()), maxInt(8, innerW-2))))
		}
		if count > shown {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  %sand %d more", currentGlyphs.Ellipsis, count-shown)))
		}
		lines = append(lines,
			"",
			textStyle.Render("They will also be removed from every playlist."),
			"",
			dimStyle.Render("Press y or enter to delete"+currentGlyphs.Separator+"esc to cancel."),
		)
	}
	return renderBox("Delete tracks", lines, width, height, colRed, colRed)
}

func (m *Model) renderDeletePlaylistModal(w, contentH int) string {
	width := modalWidth(w)
	innerW := width - 4
	height := modalHeight(contentH, 9)

	name := cleanText(m.deletePlaylist)
	if name == "" {
		name = "this playlist"
	}
	lines := []string{
		warningStyle.Render("The playlist will be deleted."),
		"",
		titleStyle.Render(truncateText("\""+name+"\"", innerW)),
		"",
		textStyle.Render("Audio files stay on disk and remain in the library."),
		"",
		dimStyle.Render("Press y or enter to delete" + currentGlyphs.Separator + "esc to cancel."),
	}
	return renderBox("Delete playlist", lines, width, height, colRed, colRed)
}

func (m *Model) renderHelpModal(w, contentH int) string {
	width := minInt(84, maxInt(52, w-8))
	innerW := width - 4
	height := modalHeight(contentH, 23)

	playback := []keyBinding{
		{currentGlyphs.UpDownWords + " / j k", "move selection"},
		{"pgup pgdn", "page up / down"},
		{"g / G", "first / last"},
		{"enter", "play / open view"},
		{"tab", "switch pane"},
		{"/", "search library"},
		{"esc", "clear search"},
		{"space", "pause / resume"},
		{"n / p", "next / previous"},
		{currentGlyphs.LeftRightWords, "seek 5 seconds"},
		{"H / L", "seek 30 seconds"},
		{"- / =", "volume down / up"},
		{"s / r", "shuffle / repeat (" + iconShuffle + " / " + iconRepeat + ")"},
		{"z", "sleep timer"},
		{"o", "sort library"},
		{"ctrl+a", "select all"},
		{"q", "save & quit"},
	}
	if mouseHintAvailable() {
		playback = append(playback[:15], append([]keyBinding{
			{"ctrl+click", "multi-select"},
			{"drag", "tracks to playlist"},
		}, playback[15:]...)...)
	}
	library := []keyBinding{
		{"a", "add music link"},
		{"P", "new playlist"},
		{"I", "import M3U"},
		{"E", "export M3U"},
		{"f", "favorite"},
		{"e", "edit tags"},
		{"M", "find duplicates"},
		{"x", "add selection"},
		{"X", "quick remove from playlist"},
		{"D / delete", "remove or delete (asks first)"},
		{"d", "downloads"},
		{"u", "rescan library"},
		{"c", "full-screen cover"},
		{"t / T", "next / prev theme"},
		{"ctrl+t", "theme list"},
	}
	if m.updateEnabled {
		library = append(library, keyBinding{"U", "update player (when available)"})
	}
	if mouseHintAvailable() {
		library = append(library, keyBinding{"mouse", "click" + currentGlyphs.Separator + iconShuffle + " " + iconRepeat + currentGlyphs.Separator + "volume"})
	}
	downloads := []keyBinding{
		{"y / enter", "confirm delete"},
		{"esc", "cancel / close"},
		{"c", "cancel download"},
		{"C", "clear finished"},
		{"a", "add link (downloads)"},
		{"?", "this help"},
		{"ctrl+c", "quit (saves session)"},
	}

	colW := maxInt(20, (innerW-3)/2)
	lines := []string{
		fitLine(sectionStyle.Render("PLAYBACK")+strings.Repeat(" ", maxInt(1, colW-8))+
			"   "+sectionStyle.Render("LIBRARY & DOWNLOADS"), innerW),
		"",
	}

	rows := maxInt(len(playback), len(library)+len(downloads))
	right := append(append([]keyBinding(nil), library...), downloads...)
	visible := maxInt(1, height-2-2)
	maxOffset := maxInt(0, rows-visible)
	offset := m.modalCursor
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	m.modalCursor = offset
	end := minInt(rows, offset+visible)
	for i := offset; i < end; i++ {
		leftCell := ""
		if i < len(playback) {
			b := playback[i]
			leftCell = keyStyle.Render(fmt.Sprintf("%-11s", b.key)) + dimStyle.Render(b.desc)
		}
		rightCell := ""
		if i < len(right) {
			b := right[i]
			rightCell = keyStyle.Render(fmt.Sprintf("%-11s", b.key)) + dimStyle.Render(b.desc)
		}
		lines = append(lines, fitLine(fitLine(leftCell, colW)+"   "+fitLine(rightCell, colW), innerW))
	}
	return renderBox("Keys & mouse", lines, width, height, colAccent, colAccent)
}

func (m *Model) renderDownloadsModal(w, contentH int) string {
	width := minInt(86, maxInt(48, w-8))
	innerW := width - 4
	m.refreshJobs()
	jobs := m.jobs
	desired := 22
	if len(jobs) == 0 {
		desired = 8
	}
	height := modalHeight(contentH, desired)
	innerH := height - 2

	if m.downloadsCur < 0 {
		m.downloadsCur = 0
	}
	if m.downloadsCur >= len(jobs) {
		m.downloadsCur = maxInt(0, len(jobs)-1)
	}

	active := m.mgr.ActiveCount()
	header := fmt.Sprintf("%d downloads", len(jobs))
	if active > 0 {
		header = fmt.Sprintf("%d downloads%s%d active", len(jobs), currentGlyphs.Separator, active)
	}
	lines := []string{
		sectionStyle.Render(header),
		"",
	}

	if len(jobs) == 0 {
		lines = append(lines, dimStyle.Render("No downloads yet."))
		lines = append(lines, dimStyle.Render("Press a to paste a Spotify, SoundCloud or YouTube link."))
		for len(lines) < innerH {
			lines = append(lines, "")
		}
		return renderBox("Downloads", lines, width, height, colAccent, colAccent)
	}

	rows := maxInt(1, innerH-5)
	offset := 0
	if m.downloadsCur >= rows {
		offset = m.downloadsCur - rows + 1
	}
	m.downloadsOff = offset
	end := minInt(len(jobs), offset+rows)
	for i := offset; i < end; i++ {
		job := jobs[i]
		icon := m.downloadStatusIcon(job.Status)
		title := cleanText(m.jobLabel(job))
		status := job.Status.String()
		if !job.Status.Terminal() && job.Progress >= 0 {
			status = fmt.Sprintf("%s %d%%", status, int(job.Progress*100))
		}
		left := icon + " " + title
		row := padBetween(left, status, innerW)
		if i == m.downloadsCur {
			lines = append(lines, selectedStyle.Render(row))
		} else {
			switch job.Status {
			case download.StatusDone:
				lines = append(lines, successStyle.Render(row))
			case download.StatusFailed:
				lines = append(lines, errorStyle.Render(row))
			case download.StatusCanceled:
				lines = append(lines, dimStyle.Render(row))
			default:
				lines = append(lines, textStyle.Render(row))
			}
		}
	}
	for len(lines) < innerH-3 {
		lines = append(lines, "")
	}

	job := jobs[m.downloadsCur]
	detail := cleanText(job.Detail)
	if job.Err != "" {
		detail = cleanText(job.Err)
	}
	if detail == "" {
		detail = job.Status.String()
	}
	lines = append(lines,
		dimStyle.Render("stage  ")+textStyle.Render(truncateText(cleanText(job.Stage), 12))+
			faintStyle.Render(currentGlyphs.Separator)+dimStyle.Render(truncateText(detail, maxInt(8, innerW-22))),
	)
	switch {
	case !job.Status.Terminal() && job.Progress >= 0:
		barW := maxInt(8, innerW-12)
		lines = append(lines, progressBar(job.Progress, barW, true)+dimStyle.Render(fmt.Sprintf("  %d%%", int(job.Progress*100))))
	case job.Warning != "":
		lines = append(lines, warningStyle.Render(truncateText("warning: "+cleanText(job.Warning), innerW)))
	case job.Status == download.StatusDone:
		lines = append(lines, dimStyle.Render("saved to "+truncateText(cleanText(job.Path), maxInt(8, innerW-9))))
	default:
		lines = append(lines, faintStyle.Render(currentGlyphs.Dash))
	}
	return renderBox("Downloads", lines, width, height, colAccent, colAccent)
}
