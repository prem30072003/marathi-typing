// Package engine is a line-by-line Go port of engine/engine.js (the Marathi
// transliteration engine used by the Chrome extension). Keep the two in sync:
// tests/parity.sh checks that both give identical suggestions.
package engine

import (
	"bufio"
	"encoding/json"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ---------------------------------------------------------------------------
// Devanagari -> Roman (used to index the lexicon)
// ---------------------------------------------------------------------------

var consMap = map[string]string{
	"क": "k", "ख": "kh", "ग": "g", "घ": "gh", "ङ": "n",
	"च": "ch", "छ": "chh", "ज": "j", "झ": "z", "ञ": "n",
	"ट": "t", "ठ": "th", "ड": "d", "ढ": "dh", "ण": "n",
	"त": "t", "थ": "th", "द": "d", "ध": "dh", "न": "n", "ऩ": "n",
	"प": "p", "फ": "ph", "ब": "b", "भ": "bh", "म": "m",
	"य": "y", "र": "r", "ऱ": "r", "ल": "l", "ळ": "l", "ऴ": "l", "व": "v",
	"श": "sh", "ष": "sh", "स": "s", "ह": "h",
	"क़": "k", "ख़": "kh", "ग़": "g", "ज़": "z", "ड़": "d", "ढ़": "dh", "फ़": "f", "य़": "y",
	"क़": "k", "ख़": "kh", "ग़": "g", "ज़": "z", "ड़": "d", "ढ़": "dh", "फ़": "f", "य़": "y",
}

var nuktaBase = map[string]string{
	"क": "क़", "ख": "ख़", "ग": "ग़", "ज": "ज़", "ड": "ड़", "ढ": "ढ़", "फ": "फ़", "य": "य़",
}

var indVowel = map[string]string{
	"अ": "a", "आ": "a", "इ": "i", "ई": "i", "उ": "u", "ऊ": "u", "ऋ": "ru",
	"ए": "e", "ऎ": "e", "ऐ": "ai", "ओ": "o", "ऒ": "o", "औ": "au",
	"ऍ": "e", "ॲ": "a", "ऑ": "o",
}

var matraMap = map[string]string{
	"ा": "a", "ि": "i", "ी": "i", "ु": "u", "ू": "u", "ृ": "ru",
	"े": "e", "ॆ": "e", "ै": "ai", "ो": "o", "ॊ": "o", "ौ": "au",
	"ॅ": "a", "ॉ": "o",
}

const (
	virama   = "्"
	anusvara = "ं"
	chandra  = "ँ"
	visarga  = "ः"
	nukta    = "़"
)

type akshara struct {
	c       []string // consonant romans
	cd      []string // consonant Devanagari
	v       string   // vowel roman, "A" = inherent schwa, "" = virama
	n, h    bool
	pending bool
}

func parseWord(word string) []*akshara {
	word = strings.ReplaceAll(strings.ReplaceAll(word, "‌", ""), "‍", "")
	var chars []string
	for _, r := range word {
		chars = append(chars, string(r))
	}
	var aks []*akshara
	var cur *akshara
	for i := 0; i < len(chars); i++ {
		ch := chars[i]
		if i+1 < len(chars) && chars[i+1] == nukta {
			if nb, ok := nuktaBase[ch]; ok {
				ch = nb
				i++
			}
		}
		if cr, ok := consMap[ch]; ok {
			if cur != nil && cur.v == "" && cur.pending {
				cur.c = append(cur.c, cr)
				cur.cd = append(cur.cd, ch)
				cur.pending = false
				cur.v = "A"
			} else {
				cur = &akshara{c: []string{cr}, cd: []string{ch}, v: "A"}
				aks = append(aks, cur)
			}
		} else if iv, ok := indVowel[ch]; ok {
			cur = &akshara{v: iv}
			aks = append(aks, cur)
		} else if mv, ok := matraMap[ch]; ok {
			if cur != nil && len(cur.c) > 0 {
				cur.v = mv
				cur.pending = false
			}
		} else if ch == virama {
			if cur != nil && len(cur.c) > 0 {
				cur.v = ""
				cur.pending = true
			}
		} else if ch == anusvara || ch == chandra {
			if cur != nil {
				cur.n = true
			}
		} else if ch == visarga {
			if cur != nil {
				cur.h = true
			}
		}
	}
	return aks
}

func schwaMask(aks []*akshara, medial, keepFinalCluster bool) []bool {
	n := len(aks)
	keep := make([]bool, n)
	for i, a := range aks {
		keep[i] = a.v == "A"
	}
	if n > 1 && keep[n-1] && len(aks[n-1].c) > 0 && !aks[n-1].n && !aks[n-1].h &&
		!(keepFinalCluster && len(aks[n-1].c) > 1) {
		keep[n-1] = false
	}
	if medial {
		for i := n - 2; i >= 1; i-- {
			a := aks[i]
			if a.v != "A" || a.n || a.h || len(a.c) != 1 {
				continue
			}
			prev, next := aks[i-1], aks[i+1]
			prevHasV := prev.v != "" && (prev.v != "A" || keep[i-1])
			nextSimple := len(next.c) == 1 || (len(next.c) == 2 && next.c[1] == "y")
			nextHasV := nextSimple && next.v != "" && (next.v != "A" || keep[i+1])
			if prevHasV && nextHasV && len(next.c) > 0 {
				keep[i] = false
			}
		}
	}
	return keep
}

var labial = map[string]bool{"p": true, "ph": true, "b": true, "bh": true, "m": true}

type romOpts struct {
	medial, finalNasal bool
	dny, ru            string
}

func romanize(aks []*akshara, o romOpts) string {
	keep := schwaMask(aks, o.medial, !o.medial)
	var out strings.Builder
	for i, a := range aks {
		cons := strings.Join(a.c, "")
		if len(a.cd) == 2 && a.cd[0] == "ज" && a.cd[1] == "ञ" {
			cons = o.dny
		} else if len(a.cd) == 2 && a.cd[0] == "क" && a.cd[1] == "ष" {
			cons = "ksh"
		}
		out.WriteString(cons)
		v := a.v
		if a.v == "A" {
			if keep[i] {
				v = "a"
			} else {
				v = ""
			}
		}
		if a.v == "ru" {
			v = o.ru
		}
		out.WriteString(v)
		if a.n {
			if i != len(aks)-1 {
				nx := aks[i+1]
				if len(nx.c) > 0 && labial[nx.c[0]] {
					out.WriteString("m")
				} else {
					out.WriteString("n")
				}
			} else if o.finalNasal {
				out.WriteString("n")
			}
		}
		if a.h && o.finalNasal {
			out.WriteString("h")
		}
	}
	return out.String()
}

// DevToRomanVariants returns the likely ways a Devanagari word is typed in Roman letters.
func DevToRomanVariants(word string) []string {
	aks := parseWord(word)
	if len(aks) == 0 {
		return nil
	}
	hasDny := strings.Contains(word, "ज्ञ")
	hasRu := strings.ContainsAny(word, "ृऋ")
	seen := map[string]bool{}
	var list []string
	add := func(o romOpts) {
		r := romanize(aks, o)
		if r != "" && !seen[r] {
			seen[r] = true
			list = append(list, r)
		}
	}
	add(romOpts{true, false, "dny", "ru"})
	add(romOpts{false, false, "dny", "ru"})
	if aks[len(aks)-1].n || strings.ContainsAny(word, "ःँ") {
		add(romOpts{true, true, "dny", "ru"})
	}
	if hasDny {
		add(romOpts{true, false, "gy", "ru"})
		add(romOpts{true, false, "gn", "ru"})
		add(romOpts{false, false, "gny", "ru"})
	}
	if hasRu {
		add(romOpts{true, false, "dny", "ri"})
	}
	if len(aks) == 1 && aks[0].v == "A" && len(aks[0].c) == 1 && !aks[0].n && !seen[aks[0].c[0]] {
		seen[aks[0].c[0]] = true
		list = append(list, aks[0].c[0])
	}
	return list
}

// ---------------------------------------------------------------------------
// Loose keys over Roman text (regex-free versions of the JS replaces)
// ---------------------------------------------------------------------------

func isVowel(c byte) bool     { return c == 'a' || c == 'e' || c == 'i' || c == 'o' || c == 'u' }
func isConsonant(c byte) bool { return c >= 'a' && c <= 'z' && !isVowel(c) }

func onlyLower(s string) string {
	s = strings.ToLower(s)
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= 'a' && s[i] <= 'z' {
			b = append(b, s[i])
		}
	}
	return string(b)
}

// c(?=[eiy]) -> s   then   c(?!h) -> k
func replaceC(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] == 'c' && i+1 < len(b) && (b[i+1] == 'e' || b[i+1] == 'i' || b[i+1] == 'y') {
			b[i] = 's'
		}
	}
	for i := range b {
		if b[i] == 'c' && !(i+1 < len(b) && b[i+1] == 'h') {
			b[i] = 'k'
		}
	}
	return string(b)
}

// collapse runs of the same character for which pred is true
func collapseRuns(s string, pred func(byte) bool) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if len(b) > 0 && pred(s[i]) && b[len(b)-1] == s[i] && (i > 0 && s[i-1] == s[i]) {
			continue
		}
		b = append(b, s[i])
	}
	return string(b)
}

// ([b-df-hj-np-tv-z])y(?![aeiou]) -> $1i
func consYToI(s string) string {
	var b []byte
	for i := 0; i < len(s); {
		if i+1 < len(s) && isConsonant(s[i]) && s[i+1] == 'y' && !(i+2 < len(s) && isVowel(s[i+2])) {
			b = append(b, s[i], 'i')
			i += 2
			continue
		}
		b = append(b, s[i])
		i++
	}
	return string(b)
}

// ([set])h -> $1
func dropHAfter(s string, set string) string {
	var b []byte
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i+1] == 'h' && strings.IndexByte(set, s[i]) >= 0 {
			b = append(b, s[i])
			i += 2
			continue
		}
		b = append(b, s[i])
		i++
	}
	return string(b)
}

// iy(?=[aeiou]) -> i
func iyBeforeVowel(s string) string {
	var b []byte
	for i := 0; i < len(s); {
		if i+2 < len(s) && s[i] == 'i' && s[i+1] == 'y' && isVowel(s[i+2]) {
			b = append(b, 'i')
			i += 2
			continue
		}
		b = append(b, s[i])
		i++
	}
	return string(b)
}

// ay(?![aeiou]) -> e
func ayToE(s string) string {
	var b []byte
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == 'a' && s[i+1] == 'y' && !(i+2 < len(s) && isVowel(s[i+2])) {
			b = append(b, 'e')
			i += 2
			continue
		}
		b = append(b, s[i])
		i++
	}
	return string(b)
}

// m(?=[consonant]) -> n
func mBeforeCons(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] == 'm' && i+1 < len(b) && isConsonant(b[i+1]) {
			b[i] = 'n'
		}
	}
	return string(b)
}

func Key1(s string) string {
	s = onlyLower(s)
	s = strings.ReplaceAll(s, "chh", "ch")
	s = strings.ReplaceAll(s, "cch", "ch")
	s = strings.ReplaceAll(s, "ksh", "x")
	s = strings.ReplaceAll(s, "ph", "f")
	s = strings.ReplaceAll(s, "jh", "z")
	s = strings.ReplaceAll(s, "w", "v")
	s = strings.ReplaceAll(s, "q", "k")
	s = replaceC(s)
	s = strings.ReplaceAll(s, "ou", "au")
	s = strings.ReplaceAll(s, "ee", "i")
	s = strings.ReplaceAll(s, "oo", "u")
	s = collapseRuns(s, isVowel)
	s = consYToI(s)
	if strings.HasSuffix(s, "aon") {
		s = s[:len(s)-3] + "av"
	} else if strings.HasSuffix(s, "ao") {
		s = s[:len(s)-2] + "av"
	}
	return s
}

func Key2(s string) string {
	s = Key1(s)
	s = strings.ReplaceAll(s, "sh", "s")
	s = strings.ReplaceAll(s, "x", "ks")
	s = dropHAfter(s, "kgcjtdpb")
	s = strings.ReplaceAll(s, "z", "j")
	s = iyBeforeVowel(s)
	if strings.HasSuffix(s, "iy") {
		s = s[:len(s)-2] + "i"
	}
	s = ayToE(s)
	s = dropHAfter(s, "lnmrv")
	s = mBeforeCons(s)
	s = strings.ReplaceAll(s, "ai", "e")
	s = strings.ReplaceAll(s, "au", "o")
	s = strings.ReplaceAll(s, "ei", "e")
	s = collapseRuns(s, isConsonant)
	if len(s) > 1 {
		s = s[:1] + strings.ReplaceAll(s[1:], "a", "")
	}
	s = collapseRuns(s, isConsonant)
	return s
}

func editDist(a, b string) float64 {
	m, n := len(a), len(b)
	if m == 0 {
		return float64(n)
	}
	if n == 0 {
		return float64(m)
	}
	prev := make([]float64, n+1)
	cur := make([]float64, n+1)
	for j := 0; j <= n; j++ {
		prev[j] = float64(j)
	}
	for i := 1; i <= m; i++ {
		cur[0] = float64(i)
		ca := a[i-1]
		for j := 1; j <= n; j++ {
			cb := b[j-1]
			sub := prev[j-1]
			if ca != cb {
				sub++
			}
			da, db := 1.0, 1.0
			if ca == 'a' || ca == 'h' {
				da = 0.5
			}
			if cb == 'a' || cb == 'h' {
				db = 0.5
			}
			del, ins := prev[j]+da, cur[j-1]+db
			if sub < del {
				if sub < ins {
					cur[j] = sub
				} else {
					cur[j] = ins
				}
			} else if del < ins {
				cur[j] = del
			} else {
				cur[j] = ins
			}
		}
		prev, cur = cur, prev
	}
	return prev[n]
}

// ---------------------------------------------------------------------------
// Roman -> Devanagari rules (fallback for words not in the lexicon)
// ---------------------------------------------------------------------------

var rCons = [][2]string{
	{"ksh", "क्ष"}, {"dny", "ज्ञ"}, {"gny", "ज्ञ"}, {"chh", "छ"}, {"Sh", "ष"}, {"shh", "ष"},
	{"kh", "ख"}, {"gh", "घ"}, {"ch", "च"}, {"jh", "झ"}, {"Th", "ठ"}, {"Dh", "ढ"},
	{"th", "थ"}, {"dh", "ध"}, {"ph", "फ"}, {"bh", "भ"}, {"sh", "श"},
	{"k", "क"}, {"g", "ग"}, {"j", "ज"}, {"z", "झ"}, {"T", "ट"}, {"D", "ड"}, {"N", "ण"},
	{"t", "त"}, {"d", "द"}, {"n", "न"}, {"p", "प"}, {"f", "फ"}, {"b", "ब"}, {"m", "म"},
	{"y", "य"}, {"r", "र"}, {"L", "ळ"}, {"l", "ल"}, {"v", "व"}, {"w", "व"}, {"s", "स"}, {"h", "ह"},
	{"x", "क्ष"}, {"q", "क"}, {"c", "क"},
}

var rVow = [][3]string{
	{"aa", "आ", "ा"}, {"ai", "ऐ", "ै"}, {"au", "औ", "ौ"}, {"ou", "औ", "ौ"}, {"ee", "ई", "ी"}, {"ii", "ई", "ी"},
	{"oo", "ऊ", "ू"}, {"uu", "ऊ", "ू"}, {"a", "अ", ""}, {"i", "इ", "ि"}, {"u", "उ", "ु"},
	{"e", "ए", "े"}, {"o", "ओ", "ो"}, {"A", "आ", "ा"}, {"I", "ई", "ी"}, {"U", "ऊ", "ू"}, {"E", "ऐ", "ै"}, {"O", "औ", "ौ"},
}

type rtok struct {
	vowel         bool
	ind, mat, dev string
	r             string
}

func onlyLetters(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			b = append(b, c)
		}
	}
	return string(b)
}

func matchVow(s string, i int) *[3]string {
	for k := range rVow {
		if strings.HasPrefix(s[i:], rVow[k][0]) {
			return &rVow[k]
		}
	}
	return nil
}

func matchCons(s string, i int) *[2]string {
	for k := range rCons {
		if strings.HasPrefix(s[i:], rCons[k][0]) {
			return &rCons[k]
		}
	}
	return nil
}

// RulesToDev spells a Roman word in Devanagari by rule.
func RulesToDev(input string) string {
	s := onlyLetters(input)
	if s == "" {
		return ""
	}
	s = strings.ToLower(s[:1]) + s[1:]
	var toks []rtok
	for i := 0; i < len(s); {
		v := matchVow(s, i)
		var c *[2]string
		if v == nil {
			c = matchCons(s, i)
		}
		if v == nil && c == nil {
			lc := strings.ToLower(s[i : i+1])
			v = matchVow(lc, 0)
			if v == nil {
				c = matchCons(lc, 0)
			}
		}
		if v != nil {
			toks = append(toks, rtok{vowel: true, ind: v[1], mat: v[2], r: strings.ToLower(v[0])})
			i += len(v[0])
		} else if c != nil {
			toks = append(toks, rtok{dev: c[1], r: strings.ToLower(c[0])})
			i += len(c[0])
		} else {
			i++
		}
	}
	var out strings.Builder
	for k := range toks {
		tk := toks[k]
		var prev, next *rtok
		if k > 0 {
			prev = &toks[k-1]
		}
		if k+1 < len(toks) {
			next = &toks[k+1]
		}
		if tk.vowel {
			last := k == len(toks)-1
			if prev != nil && !prev.vowel {
				m := tk.mat
				if last && tk.r == "a" && len(toks) > 2 {
					m = "ा"
				} else if last && tk.r == "i" {
					m = "ी"
				} else if last && tk.r == "u" {
					m = "ू"
				}
				out.WriteString(m)
			} else {
				ind := tk.ind
				if last && tk.r == "i" && prev != nil {
					ind = "ई"
				}
				out.WriteString(ind)
			}
		} else {
			if (tk.r == "n" || tk.r == "m") && prev != nil && prev.vowel && next != nil && !next.vowel {
				nr := next.r
				blocked := nr == "n" || nr == "m" || nr == "y" || nr == "r" || nr == "v" || nr == "w" || nr == "h"
				mBad := tk.r == "m" && nr != "p" && nr != "b" && nr != "bh" && nr != "ph"
				if !blocked && !mBad {
					out.WriteString("ं")
					continue
				}
			}
			out.WriteString(tk.dev)
			if next != nil && !next.vowel {
				out.WriteString("्")
			}
		}
	}
	return out.String()
}

// ---------------------------------------------------------------------------
// Chat shorthand and English loanwords
// ---------------------------------------------------------------------------

var shorthand = map[string]string{
	"mla": "मला", "tla": "तला", "tula": "तुला", "mnje": "म्हणजे", "mhnje": "म्हणजे", "mhanje": "म्हणजे",
	"kr": "कर", "krto": "करतो", "krte": "करते", "krtoy": "करतोय", "krtey": "करतेय", "krun": "करून", "kel": "केलं",
	"pn": "पण", "nhi": "नाही", "nai": "नाही", "nay": "नाय", "ny": "नाही", "n": "न", "v": "व",
	"ahe": "आहे", "ahes": "आहेस", "aahe": "आहे", "ahet": "आहेत", "hota": "होता",
	"kay": "काय", "kai": "काय", "ky": "काय", "ka": "का", "kasa": "कसा", "kashi": "कशी", "kas": "कसं",
	"kuthe": "कुठे", "kuthay": "कुठाय", "kdhi": "कधी", "kadhi": "कधी", "kiti": "किती", "kon": "कोण",
	"mi": "मी", "tu": "तू", "to": "तो", "ti": "ती", "te": "ते", "amhi": "आम्ही", "tumhi": "तुम्ही", "aapan": "आपण",
	"maza": "माझा", "mazi": "माझी", "maze": "माझे", "tuza": "तुझा", "tuzi": "तुझी", "tuze": "तुझे",
	"mazya": "माझ्या", "tuzya": "तुझ्या", "tyacha": "त्याचा", "ticha": "तिचा", "tyancha": "त्यांचा",
	"ho": "हो", "hoy": "होय", "nko": "नको", "nako": "नको", "bg": "बघ", "bagh": "बघ", "chal": "चल",
	"ata": "आता", "atta": "आत्ता", "aaj": "आज", "udya": "उद्या", "kal": "काल", "ani": "आणि", "aani": "आणि",
	"khup": "खूप", "jara": "जरा", "ekda": "एकदा", "parat": "परत", "bhari": "भारी", "mast": "मस्त",
	"zala": "झाला", "zali": "झाली", "zale": "झाले", "zal": "झालं", "jevlas": "जेवलास", "jevlis": "जेवलीस",
	"thik": "ठीक", "barobar": "बरोबर", "sagla": "सगळा", "sagle": "सगळे", "sagl": "सगळं", "kharach": "खरंच",
	"bol": "बोल", "sang": "सांग", "ye": "ये", "ja": "जा", "yeto": "येतो", "yete": "येते", "jato": "जातो", "jate": "जाते",
	"ghari": "घरी", "ghar": "घर", "ithe": "इथे", "tithe": "तिथे", "asa": "असा", "ase": "असे", "as": "असं",
	"tr": "तर", "tar": "तर", "mg": "मग", "mag": "मग", "tya": "त्या", "ya": "या", "he": "हे", "ha": "हा", "hi": "ही",
	"dada": "दादा", "tai": "ताई", "aai": "आई", "baba": "बाबा", "mitra": "मित्र", "shala": "शाळा",
	"bhetu": "भेटू", "bhetuya": "भेटूया", "bolu": "बोलू", "karu": "करू", "karuya": "करूया",
	"de": "दे", "ghe": "घे", "kar": "कर", "thamb": "थांब", "bas": "बस", "chala": "चला",
}

var english = func() map[string]string {
	src := "mobile मोबाईल|phone फोन|call कॉल|message मेसेज|whatsapp व्हॉट्सॲप|video व्हिडिओ|photo फोटो|status स्टेटस|" +
		"recharge रिचार्ज|charge चार्ज|charger चार्जर|battery बॅटरी|laptop लॅपटॉप|computer कॉम्प्युटर|internet इंटरनेट|" +
		"online ऑनलाइन|offline ऑफलाइन|password पासवर्ड|email ईमेल|link लिंक|app ॲप|school स्कूल|college कॉलेज|class क्लास|" +
		"exam एक्झाम|result रिझल्ट|marks मार्क्स|teacher टीचर|sir सर|madam मॅडम|office ऑफिस|meeting मीटिंग|boss बॉस|job जॉब|" +
		"salary सॅलरी|interview इंटरव्ह्यू|project प्रोजेक्ट|company कंपनी|team टीम|manager मॅनेजर|report रिपोर्ट|file फाईल|" +
		"bus बस|train ट्रेन|station स्टेशन|ticket तिकीट|auto ऑटो|car कार|bike बाईक|petrol पेट्रोल|road रोड|traffic ट्रॅफिक|" +
		"late लेट|time टाइम|sorry सॉरी|thanks थँक्स|thank थँक|please प्लीज|ok ओके|okay ओके|hello हॅलो|bye बाय|good गुड|" +
		"morning मॉर्निंग|night नाईट|birthday बर्थडे|party पार्टी|happy हॅपी|movie मूव्ही|hotel हॉटेल|table टेबल|doctor डॉक्टर|" +
		"hospital हॉस्पिटल|medical मेडिकल|tension टेन्शन|problem प्रॉब्लेम|plan प्लॅन|cancel कॅन्सल|confirm कन्फर्म|ready रेडी|" +
		"done डन|free फ्री|busy बिझी|room रूम|hostel हॉस्टेल|canteen कॅन्टीन|bank बँक|account अकाउंट|card कार्ड|cash कॅश|" +
		"order ऑर्डर|delivery डिलिव्हरी|shop शॉप|market मार्केट|science सायन्स|maths मॅथ्स|engineering इंजिनिअरिंग|" +
		"lecture लेक्चर|practical प्रॅक्टिकल|notes नोट्स|book बुक|pen पेन|week वीक|weekend वीकेंड|sunday संडे|monday मंडे|" +
		"cricket क्रिकेट|match मॅच|game गेम|news न्यूज|update अपडेट|stop स्टॉप|best बेस्ट|idea आयडिया|group ग्रुप|post पोस्ट|" +
		"share शेअर|like लाईक|comment कमेंट|selfie सेल्फी|camera कॅमेरा|light लाईट|full फुल|plus प्लस|cool कूल|set सेट|" +
		"ready रेडी|screen स्क्रीन|download डाउनलोड|upload अपलोड|website वेबसाईट|google गूगल|youtube यूट्यूब|instagram इन्स्टाग्राम"
	m := map[string]string{}
	for _, p := range strings.Split(src, "|") {
		i := strings.Index(p, " ")
		m[p[:i]] = p[i+1:]
	}
	return m
}()

// ---------------------------------------------------------------------------
// Engine
// ---------------------------------------------------------------------------

var (
	wExact   = 0.1
	wKey1    = 1.0
	wDist    = 1.0
	wSplit   = 2.0
	wEnglish = 2.0
	wCtx     = 1.5
)

type Engine struct {
	Words   []string
	logf    []float64
	vars    [][]string
	index   map[string][]int
	Learned map[string]map[string]int // input -> picked word -> count
	Bigram  map[string]map[string]int // previous word -> word -> count
	english map[string]bool
	user    map[string][]string
	uwords  []string
	ulogf   []float64
	uvars   [][]string
	uindex  map[string][]int
	keys    []string
	// KeepEnglish offers common English words unchanged as the first suggestion.
	KeepEnglish bool
}

func New() *Engine {
	return &Engine{index: map[string][]int{}, Learned: map[string]map[string]int{}, Bigram: map[string]map[string]int{},
		english: map[string]bool{}, user: map[string][]string{}, uindex: map[string][]int{}}
}

// Load reads a compiled lexicon ("word<TAB>log10freq<TAB>keys") or a raw list ("word<TAB>prob").
func (e *Engine) Load(r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) >= 3 {
			lf, _ := strconv.ParseFloat(f[1], 64)
			e.addCompiled(f[0], lf, strings.Split(f[2], " "))
		} else {
			p := 1e-9
			if len(f) > 1 {
				p, _ = strconv.ParseFloat(f[1], 64)
			}
			e.addWord(f[0], p)
		}
	}
	e.keys = nil
	return sc.Err()
}

// LoadEnglish reads one lowercase English word per line.
func (e *Engine) LoadEnglish(r io.Reader) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		w := strings.TrimSpace(sc.Text())
		if w != "" && w[0] != '#' {
			e.english[w] = true
		}
	}
	return sc.Err()
}

func addToIndex(index map[string][]int, keys []string, id int) {
	for _, k := range keys {
		if k == "" {
			continue
		}
		b, ok := index[k]
		if ok {
			if len(b) < 400 {
				index[k] = append(b, id)
			}
		} else {
			index[k] = []int{id}
		}
	}
}

func (e *Engine) addCompiled(w string, logf float64, keys []string) {
	id := len(e.Words)
	e.Words = append(e.Words, w)
	e.logf = append(e.logf, logf)
	e.vars = append(e.vars, nil)
	addToIndex(e.index, keys, id)
}

func (e *Engine) addWord(w string, p float64) {
	keys := WordKeys(w)
	if len(keys) == 0 {
		return
	}
	e.addCompiled(w, math.Round(math.Log(p)/math.Ln10*100)/100, keys)
}

func WordKeys(w string) []string {
	var keys []string
	seen := map[string]bool{}
	for _, v := range DevToRomanVariants(w) {
		k := Key2(v)
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

func isDevaWord(w string) bool {
	if w == "" {
		return false
	}
	for _, r := range w {
		if !((r >= 0x0900 && r <= 0x097F) || r == 0x200c || r == 0x200d) {
			return false
		}
	}
	return true
}

func trimAll(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == '\ufeff' })
}

// SetUserDict loads "My dictionary": lines "shortcut = text" or a bare Devanagari word.
func (e *Engine) SetUserDict(text string) int {
	e.user = map[string][]string{}
	e.uwords, e.ulogf, e.uvars, e.uindex = nil, nil, nil, map[string][]int{}
	seenW := map[string]bool{}
	addU := func(w string) {
		if !isDevaWord(w) || seenW[w] {
			return
		}
		keys := WordKeys(w)
		if len(keys) == 0 {
			return
		}
		seenW[w] = true
		id := len(e.uwords)
		e.uwords = append(e.uwords, w)
		e.ulogf = append(e.ulogf, -2.5)
		e.uvars = append(e.uvars, nil)
		addToIndex(e.uindex, keys, id)
	}
	n := 0
	for _, line := range strings.Split(text, "\n") {
		line = trimAll(strings.TrimSuffix(line, "\r"))
		if line == "" || line[0] == '#' {
			continue
		}
		m := strings.Index(line, "=")
		if m < 0 {
			m = strings.Index(line, "\t")
		}
		if m > 0 {
			sc := strings.ToLower(onlyLetters(line[:m]))
			val := trimAll(line[m+1:])
			if sc == "" || val == "" {
				continue
			}
			e.user[sc] = append(e.user[sc], val)
			addU(val)
			n++
		} else {
			addU(line)
			n++
		}
	}
	return n
}

func (e *Engine) Learn(input, word string) {
	k := strings.ToLower(onlyLetters(input))
	if k == "" || word == "" {
		return
	}
	m := e.Learned[k]
	if m == nil {
		m = map[string]int{}
		e.Learned[k] = m
	}
	m[word]++
}

// LearnContext remembers that word followed prev.
func (e *Engine) LearnContext(prev, word string) {
	if prev == "" || word == "" {
		return
	}
	m := e.Bigram[prev]
	if m == nil {
		if len(e.Bigram) >= 20000 {
			return
		}
		m = map[string]int{}
		e.Bigram[prev] = m
	}
	m[word]++
}

type learnedFile struct {
	V       int                       `json:"v"`
	Picks   map[string]map[string]int `json:"picks"`
	Context map[string]map[string]int `json:"context"`
}

func (e *Engine) ExportLearned() ([]byte, error) {
	return json.Marshal(learnedFile{V: 2, Picks: e.Learned, Context: e.Bigram})
}

func (e *Engine) ImportLearned(b []byte) {
	var f learnedFile
	if json.Unmarshal(b, &f) == nil && f.V == 2 {
		if f.Picks == nil {
			f.Picks = map[string]map[string]int{}
		}
		if f.Context == nil {
			f.Context = map[string]map[string]int{}
		}
		e.Learned, e.Bigram = f.Picks, f.Context
		return
	}
	L := map[string]map[string]int{}
	if json.Unmarshal(b, &L) == nil {
		e.Learned = L
	}
}

type scored struct {
	w string
	s float64
}

func sortScored(a []scored) {
	sort.SliceStable(a, func(i, j int) bool {
		if a[i].s != a[j].s {
			return a[i].s > a[j].s
		}
		return a[i].w < a[j].w
	})
}

func englishForm(low string) string {
	e := strings.ReplaceAll(low, "tion", "shan")
	if strings.HasSuffix(e, "ture") {
		e = e[:len(e)-4] + "char"
	}
	if strings.HasSuffix(e, "ge") {
		e = e[:len(e)-2] + "j"
	}
	if strings.HasSuffix(e, "ce") {
		e = e[:len(e)-2] + "s"
	}
	n := len(e)
	if n >= 4 && e[n-1] == 'e' && !strings.ContainsRune("aeiouy", rune(e[n-2])) &&
		isVowel(e[n-3]) && !isVowel(e[n-4]) {
		e = e[:n-1]
	}
	if e == low {
		return ""
	}
	return e
}

func (e *Engine) rank(low string) []scored {
	sc := e.rankKey(low, 0)
	if en := englishForm(low); en != "" {
		sc = append(sc, e.rankKey(en, wEnglish)...)
		sortScored(sc)
	}
	return sc
}

func scoreBucket(out []scored, bucket []int, words []string, logf []float64, vars [][]string, low, k1 string, penalty float64) []scored {
	for _, id := range bucket {
		vs := vars[id]
		if vs == nil {
			vs = DevToRomanVariants(words[id])
			vars[id] = vs
		}
		best := -99.0
		for _, v := range vs {
			s := 0.0
			if v == low {
				s += wExact
			}
			vk1 := Key1(v)
			if vk1 == k1 {
				s += wKey1
			}
			s -= wDist * editDist(k1, vk1)
			if s > best {
				best = s
			}
		}
		out = append(out, scored{words[id], logf[id] + best - penalty})
	}
	return out
}

func (e *Engine) rankKey(low string, penalty float64) []scored {
	k1, k2 := Key1(low), Key2(low)
	var out []scored
	out = scoreBucket(out, e.index[k2], e.Words, e.logf, e.vars, low, k1, penalty)
	out = scoreBucket(out, e.uindex[k2], e.uwords, e.ulogf, e.uvars, low, k1, penalty)
	sortScored(out)
	return out
}

func (e *Engine) compound(low string) *scored {
	if len(low) < 7 {
		return nil
	}
	var best *scored
	for i := 3; i <= len(low)-3; i++ {
		a := e.rank(low[:i])
		if len(a) == 0 {
			continue
		}
		b := e.rank(low[i:])
		if len(b) == 0 {
			continue
		}
		s := a[0].s + b[0].s - wSplit
		if best == nil || s > best.s {
			best = &scored{a[0].w + b[0].w, s}
		}
	}
	return best
}

// IsEnglish reports whether the input is a common English word.
func (e *Engine) IsEnglish(input string) bool { return e.english[strings.ToLower(onlyLetters(input))] }

var caps = map[byte]string{'T': "ट", 'D': "ड", 'N': "ण", 'L': "ळ"}

// Suggest returns up to max Devanagari candidates for a Roman word, best first.
// prev is the previous word (Devanagari) for context, or "".
func (e *Engine) Suggest(input string, max int, prev string) []string {
	if max <= 0 {
		max = 6
	}
	raw := onlyLetters(input)
	if raw == "" {
		return nil
	}
	low := strings.ToLower(raw)
	sc := e.rank(low)
	if comp := e.compound(low); comp != nil {
		sc = append(sc, *comp)
	}
	var want []string
	for ci := 1; ci < len(raw); ci++ {
		if d, ok := caps[raw[ci]]; ok {
			want = append(want, d)
		}
	}
	if len(want) > 0 {
		for si := range sc {
			for _, wd := range want {
				if strings.Contains(sc[si].w, wd) {
					sc[si].s += 1.5
				} else {
					sc[si].s += -4
				}
			}
		}
	}
	if prev != "" {
		if bg := e.Bigram[prev]; bg != nil {
			for i := range sc {
				if c := bg[sc[i].w]; c > 0 {
					if c > 5 {
						c = 5
					}
					sc[i].s += wCtx * float64(c)
				}
			}
		}
	}
	sortScored(sc)

	var out []string
	seen := map[string]bool{}
	push := func(w string) {
		if w != "" && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	for _, u := range e.user[low] {
		push(u)
	}
	if lm := e.Learned[low]; lm != nil {
		ls := make([]string, 0, len(lm))
		for w := range lm {
			ls = append(ls, w)
		}
		sort.Slice(ls, func(i, j int) bool {
			if lm[ls[i]] != lm[ls[j]] {
				return lm[ls[i]] > lm[ls[j]]
			}
			return ls[i] < ls[j]
		})
		for _, w := range ls {
			push(w)
		}
	}
	eng := len(want) == 0 && e.english[low]
	if eng && e.KeepEnglish {
		push(raw)
	}
	if len(want) == 0 {
		if w, ok := shorthand[low]; ok {
			push(w)
		}
		if w, ok := english[low]; ok {
			push(w)
		}
	}
	for c := 0; c < len(sc) && len(out) < max-1; c++ {
		push(sc[c].w)
	}
	push(RulesToDev(raw))
	if len(out) > max {
		out = out[:max]
	}
	if eng && !seen[raw] && max > 1 {
		at := 1
		if len(out) < 1 {
			at = len(out)
		}
		out = append(out[:at], append([]string{raw}, out[at:]...)...)
		if len(out) > max {
			out = out[:max]
		}
	}
	return out
}

// Complete returns up to n longer, frequent words that start like the input.
func (e *Engine) Complete(input string, n int, exclude []string) []string {
	raw := onlyLetters(input)
	if len(raw) < 3 || n <= 0 {
		return nil
	}
	k2 := Key2(strings.ToLower(raw))
	if len(k2) < 2 {
		return nil
	}
	if e.keys == nil {
		e.keys = make([]string, 0, len(e.index))
		for k := range e.index {
			e.keys = append(e.keys, k)
		}
		sort.Strings(e.keys)
	}
	lo := sort.SearchStrings(e.keys, k2)
	seen := map[string]bool{}
	for _, x := range exclude {
		seen[x] = true
	}
	var cand []scored
	for i := lo; i < len(e.keys) && i < lo+4000; i++ {
		k := e.keys[i]
		if !strings.HasPrefix(k, k2) {
			break
		}
		if len(k) < len(k2)+1 {
			continue
		}
		for _, id := range e.index[k] {
			w := e.Words[id]
			if e.logf[id] < -5.5 || seen[w] {
				continue
			}
			seen[w] = true
			cand = append(cand, scored{w, e.logf[id]})
		}
	}
	sortScored(cand)
	var out []string
	for c := 0; c < len(cand) && len(out) < n; c++ {
		out = append(out, cand[c].w)
	}
	return out
}
