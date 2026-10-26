package ui

import "unicode"

// cyrillicShortcuts maps the Russian ЙЦУКЕН layout onto the physical keys used
// by the English shortcuts. Special keys (enter, esc, delete, arrows, …) are
// already layout-independent.
var cyrillicShortcuts = map[rune]rune{
	'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y',
	'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p', 'х': '[', 'ъ': ']',
	'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h',
	'о': 'j', 'л': 'k', 'д': 'l', 'ж': ';', 'э': '\'',
	'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n',
	'ь': 'm', 'б': ',', 'ю': '.', 'ё': '`',
}

// normalizeShortcut translates a single Cyrillic rune to its English-layout
// counterpart. The physical "/" key is "." in the Russian layout.
func normalizeShortcut(key string) string {
	runes := []rune(key)
	if len(runes) != 1 {
		return key
	}
	r := runes[0]
	if mapped, ok := cyrillicShortcuts[unicode.ToLower(r)]; ok {
		if unicode.IsUpper(r) {
			mapped = unicode.ToUpper(mapped)
		}
		return string(mapped)
	}
	if r == '.' {
		return "/"
	}
	return key
}
