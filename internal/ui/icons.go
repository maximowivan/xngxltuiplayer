package ui

// UI symbols live in the project and use single-cell Unicode characters that
// are widely available. -ascii switches to plain-text labels for terminals or
// fonts that cannot render them.
var (
	iconLogo        = "♪"
	iconShuffle     = "⇄"
	iconRepeat      = "↻"
	iconCover       = "▣"
	iconTheme       = "◐"
	iconDelete      = "✕"
	iconPlay        = "▶"
	iconPause       = "‖"
	iconPauseButton = "▮ ▮"
	iconPrev        = "◀◀"
	iconNext        = "▶▶"
)

// SetASCIIIcons replaces Unicode symbols with short ASCII labels.
func SetASCIIIcons(on bool) {
	if on {
		iconLogo = "*"
		iconShuffle = "SHF"
		iconRepeat = "RPT"
		iconCover = "[C]"
		iconTheme = "THM"
		iconDelete = "DEL"
		iconPlay = ">"
		iconPause = "||"
		iconPauseButton = "| |"
		iconPrev = "<<"
		iconNext = ">>"
		return
	}
	iconLogo = "♪"
	iconShuffle = "⇄"
	iconRepeat = "↻"
	iconCover = "▣"
	iconTheme = "◐"
	iconDelete = "✕"
	iconPlay = "▶"
	iconPause = "‖"
	iconPauseButton = "▮ ▮"
	iconPrev = "◀◀"
	iconNext = "▶▶"
}
