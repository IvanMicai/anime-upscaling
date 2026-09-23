package dualaudio

import (
	"path"
	"sort"
	"strings"
)

// Language describes the audio language a file name declares.
type Language struct {
	Tag   string `json:"tag"`   // as written in the file name, lower-cased: "pt-br"
	ISO3  string `json:"iso3"`  // ISO 639-2, what Matroska wants: "por"
	Title string `json:"title"` // track title: "Português (BR)"
}

// languages maps the tags people actually put in file names to ISO 639-2 and a
// display title. It is a closed list on purpose: the tag is the last dot
// segment before the extension, and release names are full of short segments
// that are not languages ("web", "x264", "v2", "bd"). Guessing from shape alone
// would pair "show.web.mkv" with "show.bd.mkv".
var languages = map[string]Language{}

func init() {
	for _, l := range []struct {
		iso3, title string
		tags        []string
	}{
		{"por", "Português (BR)", []string{"pt-br", "ptbr", "pt_br", "br"}},
		{"por", "Português (PT)", []string{"pt-pt", "ptpt", "pt_pt"}},
		{"por", "Português", []string{"pt", "por"}},
		{"eng", "English", []string{"en", "eng", "en-us", "en-gb", "en_us", "en_gb"}},
		{"jpn", "日本語", []string{"ja", "jp", "jpn", "ja-jp"}},
		{"spa", "Español (Latinoamérica)", []string{"es-la", "es-419", "es-mx", "es_la", "lat"}},
		{"spa", "Español", []string{"es", "spa", "es-es"}},
		{"fre", "Français", []string{"fr", "fre", "fra", "fr-fr"}},
		{"ger", "Deutsch", []string{"de", "ger", "deu", "de-de"}},
		{"ita", "Italiano", []string{"it", "ita"}},
		{"kor", "한국어", []string{"ko", "kor", "kr"}},
		{"chi", "中文", []string{"zh", "chi", "zho", "zh-cn", "zh-tw", "cn"}},
		{"rus", "Русский", []string{"ru", "rus"}},
		{"ara", "العربية", []string{"ar", "ara"}},
		{"hin", "हिन्दी", []string{"hi", "hin"}},
		{"pol", "Polski", []string{"pl", "pol"}},
		{"dut", "Nederlands", []string{"nl", "dut", "nld"}},
		{"tur", "Türkçe", []string{"tr", "tur"}},
		{"tha", "ไทย", []string{"th", "tha"}},
		{"ind", "Bahasa Indonesia", []string{"id", "ind"}},
	} {
		for _, tag := range l.tags {
			languages[tag] = Language{Tag: tag, ISO3: l.iso3, Title: l.title}
		}
	}
}

// SplitLanguage splits "dir/name.pt-br.mp4" into the stem "dir/name" and its
// language. ok is false when the name carries no known language tag.
func SplitLanguage(rel string) (stem string, lang Language, ok bool) {
	ext := path.Ext(rel)
	noExt := strings.TrimSuffix(rel, ext)
	i := strings.LastIndexAny(noExt, "._ ")
	if i <= strings.LastIndex(noExt, "/") || i == len(noExt)-1 {
		return noExt, Language{}, false
	}
	tag := strings.ToLower(noExt[i+1:])
	lang, ok = languages[tag]
	if !ok {
		return noExt, Language{}, false
	}
	stem = strings.TrimRight(noExt[:i], "._ -")
	return stem, lang, stem != "" && !strings.HasSuffix(stem, "/")
}

// NamePair is two files that are the same episode in two languages.
type NamePair struct {
	A      string   `json:"a"` // paths relative to the source folder
	B      string   `json:"b"`
	LangA  Language `json:"lang_a"`
	LangB  Language `json:"lang_b"`
	Output string   `json:"output"` // where the merged file goes, relative to the merged folder
	// Exists is set by callers that know the merged folder: the output is
	// already there, so a job would skip this pair.
	Exists bool `json:"exists,omitempty"`
}

// Unpaired is a selected file that could not be paired, and why.
type Unpaired struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

// PairByName groups files whose names differ only by their language tag:
// "ep01.pt-br.mp4" + "ep01.en.mkv" -> "ep01.mkv". The two may live in different
// folders; the output lands in their common ancestor.
//
// Matching is on the file NAME, not on the folder: the point of selecting two
// folders is that each holds one language. A stem claimed by more than two
// files, or by two files of the same language, is ambiguous and is reported
// rather than guessed at.
func PairByName(files []string) ([]NamePair, []Unpaired) {
	type member struct {
		rel  string
		lang Language
	}
	groups := map[string][]member{}
	var unpaired []Unpaired
	for _, f := range files {
		stem, lang, ok := SplitLanguage(f)
		if !ok {
			unpaired = append(unpaired, Unpaired{f, "no language tag in the name (expected e.g. name.pt-br.mp4)"})
			continue
		}
		key := strings.ToLower(path.Base(stem))
		groups[key] = append(groups[key], member{f, lang})
	}

	var pairs []NamePair
	for _, g := range groups {
		switch {
		case len(g) == 1:
			unpaired = append(unpaired, Unpaired{g[0].rel, "no file with the same name in another language"})
		case len(g) > 2:
			for _, m := range g {
				unpaired = append(unpaired, Unpaired{m.rel, "more than two files share this name"})
			}
		case g[0].lang.ISO3 == g[1].lang.ISO3 && g[0].lang.Tag == g[1].lang.Tag:
			for _, m := range g {
				unpaired = append(unpaired, Unpaired{m.rel, "both files declare the same language"})
			}
		default:
			sort.Slice(g, func(i, j int) bool { return g[i].rel < g[j].rel })
			stem, _, _ := SplitLanguage(g[0].rel)
			out := path.Join(commonDir(g[0].rel, g[1].rel), path.Base(stem)+".mkv")
			pairs = append(pairs, NamePair{A: g[0].rel, B: g[1].rel, LangA: g[0].lang, LangB: g[1].lang, Output: out})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Output < pairs[j].Output })
	sort.Slice(unpaired, func(i, j int) bool { return unpaired[i].File < unpaired[j].File })
	return pairs, unpaired
}

// commonDir is the deepest directory containing both relative paths.
func commonDir(a, b string) string {
	as, bs := strings.Split(path.Dir(a), "/"), strings.Split(path.Dir(b), "/")
	var out []string
	for i := 0; i < len(as) && i < len(bs) && as[i] == bs[i]; i++ {
		if as[i] != "." {
			out = append(out, as[i])
		}
	}
	return strings.Join(out, "/")
}
