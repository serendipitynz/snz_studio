// Package search ports the Japanese search tokenization used by the original
// Node backend (backend/src/lib/searchText.ts) to Go.
//
// segmenter.go is a faithful port of TinySegmenter 0.2 by Taku Kudo
// (https://chasen.org/~taku/software/TinySegmenter/), the exact model and
// algorithm bundled as the npm "tiny-segmenter" package the backend imports.
// The model tables and scoring loop mirror lib/index.js so that segmentation
// output matches byte-for-byte; parity is asserted by searchtext_test.go
// against golden output generated from the JS implementation.
package search

// ctype classifies a single rune into TinySegmenter's character type.
// Order matches the JS pattern iteration order: M, H, I, K, A, N, else O.
// Rune literals are used so the Go compiler resolves the exact code points
// from the same literal characters the JS regexps use.
func ctype(r rune) string {
	switch {
	case isM(r):
		return "M"
	case (r >= '一' && r <= '龠') || r == '々' || r == '〆' || r == 'ヵ' || r == 'ヶ':
		return "H"
	case r >= 'ぁ' && r <= 'ん':
		return "I"
	case (r >= 'ァ' && r <= 'ヴ') || r == 'ー' || (r >= 'ｱ' && r <= 'ﾝ') || r == 'ﾞ' || r == 'ｰ':
		return "K"
	case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= 'ａ' && r <= 'ｚ') || (r >= 'Ａ' && r <= 'Ｚ'):
		return "A"
	case (r >= '0' && r <= '9') || (r >= '０' && r <= '９'):
		return "N"
	default:
		return "O"
	}
}

func isM(r rune) bool {
	switch r {
	case '一', '二', '三', '四', '五', '六', '七', '八', '九', '十', '百', '千', '万', '億', '兆':
		return true
	}
	return false
}

const bias = -332

// Segment splits the input into morpheme-like tokens, mirroring
// TinySegmenter.prototype.segment in lib/index.js exactly.
func Segment(input string) []string {
	if input == "" {
		return []string{}
	}

	runes := []rune(input)
	result := make([]string, 0, len(runes))

	seg := make([]string, 0, len(runes)+6)
	ctypes := make([]string, 0, len(runes)+6)
	seg = append(seg, "B3", "B2", "B1")
	ctypes = append(ctypes, "O", "O", "O")
	for _, r := range runes {
		seg = append(seg, string(r))
		ctypes = append(ctypes, ctype(r))
	}
	seg = append(seg, "E1", "E2", "E3")
	ctypes = append(ctypes, "O", "O", "O")

	word := seg[3]
	p1, p2, p3 := "U", "U", "U"

	for i := 4; i < len(seg)-3; i++ {
		score := bias
		w1, w2, w3, w4, w5, w6 := seg[i-3], seg[i-2], seg[i-1], seg[i], seg[i+1], seg[i+2]
		c1, c2, c3, c4, c5, c6 := ctypes[i-3], ctypes[i-2], ctypes[i-1], ctypes[i], ctypes[i+1], ctypes[i+2]

		score += up1[p1]
		score += up2[p2]
		score += up3[p3]
		score += bp1[p1+p2]
		score += bp2[p2+p3]
		score += uw1[w1]
		score += uw2[w2]
		score += uw3[w3]
		score += uw4[w4]
		score += uw5[w5]
		score += uw6[w6]
		score += bw1[w2+w3]
		score += bw2[w3+w4]
		score += bw3[w4+w5]
		score += tw1[w1+w2+w3]
		score += tw2[w2+w3+w4]
		score += tw3[w3+w4+w5]
		score += tw4[w4+w5+w6]
		score += uc1[c1]
		score += uc2[c2]
		score += uc3[c3]
		score += uc4[c4]
		score += uc5[c5]
		score += uc6[c6]
		score += bc1[c2+c3]
		score += bc2[c3+c4]
		score += bc3[c4+c5]
		score += tc1[c1+c2+c3]
		score += tc2[c2+c3+c4]
		score += tc3[c3+c4+c5]
		score += tc4[c4+c5+c6]
		score += uq1[p1+c1]
		score += uq2[p2+c2]
		score += uq3[p3+c3]
		score += bq1[p2+c2+c3]
		score += bq2[p2+c3+c4]
		score += bq3[p3+c2+c3]
		score += bq4[p3+c3+c4]
		score += tq1[p2+c1+c2+c3]
		score += tq2[p2+c2+c3+c4]
		score += tq3[p3+c1+c2+c3]
		score += tq4[p3+c2+c3+c4]

		p := "O"
		if score > 0 {
			result = append(result, word)
			word = ""
			p = "B"
		}
		p1, p2, p3 = p2, p3, p
		word += seg[i]
	}
	result = append(result, word)

	return result
}
