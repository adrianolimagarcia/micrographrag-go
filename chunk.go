package micrographrag

import "strings"

func ChunkText(text string, maxRunes, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	r := []rune(text)
	if maxRunes <= 0 || len(r) <= maxRunes {
		return []string{text}
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxRunes {
		overlap = maxRunes / 10
	}

	var out []string
	start := 0
	for start < len(r) {
		end := start + maxRunes
		if end >= len(r) {
			end = len(r)
		} else {
			minCut := start + maxRunes/2
			cut := findCut(r, minCut, end)
			if cut > start {
				end = cut
			}
		}
		piece := strings.TrimSpace(string(r[start:end]))
		if piece != "" {
			out = append(out, piece)
		}
		if end == len(r) {
			break
		}
		start = end - overlap
		if start < 0 {
			start = 0
		}
	}
	return out
}

func findCut(r []rune, min, end int) int {
	for _, sep := range []rune{'\n', '.', '!', '?', ';', ':', ' '} {
		for i := end - 1; i >= min; i-- {
			if r[i] == sep {
				return i + 1
			}
		}
	}
	return end
}
