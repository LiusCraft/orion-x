// Package language provides a unified, hardcoded language registry.
// It is the single source of truth for language codes and names system-wide.
package language

import "strings"

// Code is a ISO 639-1 language code constant.
type Code string

const (
	ZH Code = "zh"
	EN Code = "en"
	JA Code = "ja"
	KO Code = "ko"
	RU Code = "ru"
	FR Code = "fr"
	DE Code = "de"
	ES Code = "es"
	PT Code = "pt"
	AR Code = "ar"
	IT Code = "it"
	NL Code = "nl"
	PL Code = "pl"
	TR Code = "tr"
	VI Code = "vi"
	TH Code = "th"
	ID Code = "id"
)

// Info pairs a language code with its display name.
type Info struct {
	Code Code   `json:"code"`
	Name string `json:"name"`
}

// registry maps every known Code to its display name.
var registry = map[Code]string{
	ZH: "中文",
	EN: "English",
	JA: "日本語",
	KO: "한국어",
	RU: "Русский",
	FR: "Français",
	DE: "Deutsch",
	ES: "Español",
	PT: "Português",
	AR: "العربية",
	IT: "Italiano",
	NL: "Nederlands",
	PL: "Polski",
	TR: "Türkçe",
	VI: "Tiếng Việt",
	TH: "ไทย",
	ID: "Bahasa Indonesia",
}

// All returns every known language, in no particular order.
func All() []Info {
	out := make([]Info, 0, len(registry))
	for code, name := range registry {
		out = append(out, Info{Code: code, Name: name})
	}
	return out
}

// Get returns the Info for a Code, or nil if unknown.
func Get(code Code) *Info {
	name, ok := registry[code]
	if !ok {
		return nil
	}
	return &Info{Code: code, Name: name}
}

// Exists reports whether code is a known language.
func Exists(code Code) bool {
	_, ok := registry[code]
	return ok
}

// normalizer maps common raw strings to canonical Code values.
var normalizer = map[string]Code{
	"zh":               ZH,
	"zh-cn":            ZH,
	"zh-tw":            ZH,
	"zh-hk":            ZH,
	"chinese":          ZH,
	"中文":               ZH,
	"en":               EN,
	"en-us":            EN,
	"en-gb":            EN,
	"english":          EN,
	"ja":               JA,
	"japanese":         JA,
	"日本語":              JA,
	"ko":               KO,
	"korean":           KO,
	"한국어":              KO,
	"ru":               RU,
	"russian":          RU,
	"Русский":          RU,
	"fr":               FR,
	"french":           FR,
	"français":         FR,
	"de":               DE,
	"german":           DE,
	"deutsch":          DE,
	"es":               ES,
	"spanish":          ES,
	"español":          ES,
	"pt":               PT,
	"portuguese":       PT,
	"português":        PT,
	"ar":               AR,
	"arabic":           AR,
	"العربية":          AR,
	"it":               IT,
	"italian":          IT,
	"italiano":         IT,
	"nl":               NL,
	"dutch":            NL,
	"nederlands":       NL,
	"pl":               PL,
	"polish":           PL,
	"polski":           PL,
	"tr":               TR,
	"turkish":          TR,
	"türkçe":           TR,
	"vi":               VI,
	"vietnamese":       VI,
	"tiếng việt":       VI,
	"th":               TH,
	"thai":             TH,
	"ไทย":              TH,
	"id":               ID,
	"indonesian":       ID,
	"bahasa indonesia": ID,
}

// Normalize maps a raw language string to a canonical Code.
// Unknown inputs are passed through as-is so callers can still attempt to use them.
func Normalize(raw string) Code {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if c, ok := normalizer[lower]; ok {
		return c
	}
	return Code(strings.ToLower(raw))
}

// Match reports whether want is matched by any entry in supported.
// Matching uses prefix logic: "zh-CN" matches "zh".
func Match(want string, supported []Code) bool {
	if len(supported) == 0 {
		return false
	}
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	wantLower := strings.ToLower(want)
	for _, s := range supported {
		sc := strings.ToLower(string(s))
		if strings.HasPrefix(wantLower, sc) || strings.HasPrefix(sc, wantLower) {
			return true
		}
	}
	return false
}

// Codes converts raw strings to Code values, filtering out unknown codes.
func Codes(raw []string) []Code {
	out := make([]Code, 0, len(raw))
	for _, r := range raw {
		c := Normalize(r)
		if c != "" && Exists(c) {
			out = append(out, c)
		}
	}
	return out
}
