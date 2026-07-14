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
	Code       Code    `json:"code"`
	Name       string  `json:"name"`
	ParentCode *string `json:"parent_code,omitempty"`
	Children   []*Info `json:"children,omitempty"`
}

// registry maps every known Code to its display name and optional parent.
var registry = map[Code]*Info{
	ZH: {Code: ZH, Name: "中文"},
	EN: {Code: EN, Name: "English"},
	JA: {Code: JA, Name: "日本語"},
	KO: {Code: KO, Name: "한국어"},
	RU: {Code: RU, Name: "Русский"},
	FR: {Code: FR, Name: "Français"},
	DE: {Code: DE, Name: "Deutsch"},
	ES: {Code: ES, Name: "Español"},
	PT: {Code: PT, Name: "Português"},
	AR: {Code: AR, Name: "العربية"},
	IT: {Code: IT, Name: "Italiano"},
	NL: {Code: NL, Name: "Nederlands"},
	PL: {Code: PL, Name: "Polski"},
	TR: {Code: TR, Name: "Türkçe"},
	VI: {Code: VI, Name: "Tiếng Việt"},
	TH: {Code: TH, Name: "ไทย"},
	ID: {Code: ID, Name: "Bahasa Indonesia"},

	// Dialects / regional variants
	"zh-CN": {Code: "zh-CN", Name: "中文（简体）", ParentCode: strPtr("zh")},
	"zh-TW": {Code: "zh-TW", Name: "中文（繁体）", ParentCode: strPtr("zh")},
	"zh-HK": {Code: "zh-HK", Name: "中文（香港）", ParentCode: strPtr("zh")},
	"yue":        {Code: "yue", Name: "粤语", ParentCode: strPtr("zh")},
	"zh-minnan":  {Code: "zh-minnan", Name: "闽南语", ParentCode: strPtr("zh")},
	"zh-dongbei": {Code: "zh-dongbei", Name: "东北话", ParentCode: strPtr("zh")},
	"zh-henan":   {Code: "zh-henan", Name: "河南话", ParentCode: strPtr("zh")},
	"zh-hunan":   {Code: "zh-hunan", Name: "湖南话", ParentCode: strPtr("zh")},
	"zh-shaanxi": {Code: "zh-shaanxi", Name: "陕西话", ParentCode: strPtr("zh")},
	"zh-shandong": {Code: "zh-shandong", Name: "山东话", ParentCode: strPtr("zh")},
	"zh-sichuan":  {Code: "zh-sichuan", Name: "四川话", ParentCode: strPtr("zh")},
	"zh-anhui":    {Code: "zh-anhui", Name: "安徽话", ParentCode: strPtr("zh")},
	"en-US": {Code: "en-US", Name: "英语（美式）", ParentCode: strPtr("en")},
	"en-GB": {Code: "en-GB", Name: "英语（英式）", ParentCode: strPtr("en")},
	"en-AU": {Code: "en-AU", Name: "英语（澳洲）", ParentCode: strPtr("en")},
	"fr-FR": {Code: "fr-FR", Name: "法语（法国）", ParentCode: strPtr("fr")},
	"fr-CA": {Code: "fr-CA", Name: "法语（加拿大）", ParentCode: strPtr("fr")},
	"de-DE": {Code: "de-DE", Name: "德语（德国）", ParentCode: strPtr("de")},
	"es-ES": {Code: "es-ES", Name: "西班牙语（西班牙）", ParentCode: strPtr("es")},
	"es-MX": {Code: "es-MX", Name: "西班牙语（墨西哥）", ParentCode: strPtr("es")},
	"pt-BR": {Code: "pt-BR", Name: "葡萄牙语（巴西）", ParentCode: strPtr("pt")},
	"pt-PT": {Code: "pt-PT", Name: "葡萄牙语（葡萄牙）", ParentCode: strPtr("pt")},
	"ar-SA": {Code: "ar-SA", Name: "阿拉伯语（沙特）", ParentCode: strPtr("ar")},
	"it-IT": {Code: "it-IT", Name: "意大利语（意大利）", ParentCode: strPtr("it")},
	"nl-NL": {Code: "nl-NL", Name: "荷兰语（荷兰）", ParentCode: strPtr("nl")},
	"th-TH": {Code: "th-TH", Name: "泰语（泰国）", ParentCode: strPtr("th")},
}

func strPtr(s string) *string { return &s }

// All returns every known language, in no particular order.
func All() []*Info {
	out := make([]*Info, 0, len(registry))
	for _, info := range registry {
		// Build children links
		infoCopy := *info
		infoCopy.Children = Children(info.Code)
		out = append(out, &infoCopy)
	}
	return out
}

// Get returns the Info for a Code, or nil if unknown.
func Get(code Code) *Info {
	info, ok := registry[code]
	if !ok {
		return nil
	}
	got := *info
	got.Children = Children(code)
	return &got
}

// Children returns child languages for a parent code.
func Children(parentCode Code) []*Info {
	var children []*Info
	for code, info := range registry {
		if info.ParentCode != nil && Code(*info.ParentCode) == parentCode {
			child := *info
			child.Children = Children(code)
			children = append(children, &child)
		}
	}
	return children
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
