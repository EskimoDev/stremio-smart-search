package catalog

import (
	"errors"
	"slices"
)

var errAliasChoice = errors.New("choose a listed language and region for alternative titles")

// Language describes one alternative-title choice. IMDb's akas data does not use
// locale identifiers: most rows carry only a region (ISO 3166-style, plus some
// historical codes such as SUHH) and leave language as \N. A row is accepted when
// its language is one of Codes, or when it has no language and its region is one
// where this language is the usual one (Regions). Multilingual regions (Shared,
// e.g. CA, BE, CH) only count when the row names the language explicitly.
type Language struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Codes   []string `json:"-"`
	Regions []string `json:"-"`
	Shared  []string `json:"-"`
	// Choices lists the regions a user may pick as a regional preference.
	Choices []Region `json:"regions,omitempty"`
}

type Region struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var regionNames = map[string]string{
	"US": "United States", "GB": "United Kingdom", "AU": "Australia", "NZ": "New Zealand", "IE": "Ireland", "CA": "Canada", "IN": "India", "XWW": "Worldwide",
	"FR": "France", "BE": "Belgium", "CH": "Switzerland", "LU": "Luxembourg",
	"ES": "Spain", "MX": "Mexico", "AR": "Argentina", "CO": "Colombia", "CL": "Chile", "PE": "Peru", "VE": "Venezuela", "UY": "Uruguay", "EC": "Ecuador",
	"DE": "Germany", "AT": "Austria", "IT": "Italy", "BR": "Brazil", "PT": "Portugal", "NL": "Netherlands",
	"CN": "China", "TW": "Taiwan", "HK": "Hong Kong", "SG": "Singapore",
	"SE": "Sweden", "FI": "Finland", "NO": "Norway", "DK": "Denmark",
}

func regions(ids ...string) []Region {
	out := make([]Region, 0, len(ids))
	for _, id := range ids {
		out = append(out, Region{ID: id, Name: regionNames[id]})
	}
	return out
}

// Languages are offered in the settings page. IDs are stored in snapshots; keep
// them stable. Region choices are only listed where a regional title commonly
// differs (e.g. Spain vs Latin America, France vs Québec).
func Languages() []Language {
	return []Language{
		{ID: "en", Name: "English", Codes: []string{"en"}, Regions: []string{"US", "GB", "AU", "NZ", "IE", "XWW"}, Shared: []string{"CA", "IN"}, Choices: regions("US", "GB", "AU", "CA", "IN")},
		{ID: "fr", Name: "French", Codes: []string{"fr"}, Regions: []string{"FR"}, Shared: []string{"BE", "CA", "CH", "LU"}, Choices: regions("FR", "CA", "BE", "CH")},
		{ID: "es", Name: "Spanish", Codes: []string{"es"}, Regions: []string{"ES", "MX", "AR", "CO", "CL", "PE", "VE", "UY", "EC", "BO", "PY", "CR", "GT", "DO", "CU", "PA", "SV", "HN", "NI", "PR"}, Choices: regions("ES", "MX", "AR", "CO", "CL", "PE", "VE", "UY")},
		{ID: "de", Name: "German", Codes: []string{"de"}, Regions: []string{"DE", "AT", "DDDE", "XWG"}, Shared: []string{"CH", "LU"}, Choices: regions("DE", "AT", "CH")},
		{ID: "it", Name: "Italian", Codes: []string{"it"}, Regions: []string{"IT"}, Shared: []string{"CH"}},
		{ID: "pt", Name: "Portuguese", Codes: []string{"pt"}, Regions: []string{"BR", "PT"}, Choices: regions("BR", "PT")},
		{ID: "nl", Name: "Dutch", Codes: []string{"nl"}, Regions: []string{"NL"}, Shared: []string{"BE"}},
		{ID: "sv", Name: "Swedish", Codes: []string{"sv"}, Regions: []string{"SE"}, Shared: []string{"FI"}},
		{ID: "da", Name: "Danish", Codes: []string{"da"}, Regions: []string{"DK"}},
		{ID: "no", Name: "Norwegian", Codes: []string{"no", "nb", "nn"}, Regions: []string{"NO"}},
		{ID: "fi", Name: "Finnish", Codes: []string{"fi"}, Regions: []string{"FI"}},
		{ID: "pl", Name: "Polish", Codes: []string{"pl"}, Regions: []string{"PL"}},
		{ID: "cs", Name: "Czech", Codes: []string{"cs"}, Regions: []string{"CZ"}, Shared: []string{"CSHH"}},
		{ID: "sk", Name: "Slovak", Codes: []string{"sk"}, Regions: []string{"SK"}, Shared: []string{"CSHH"}},
		{ID: "hu", Name: "Hungarian", Codes: []string{"hu"}, Regions: []string{"HU"}},
		{ID: "ro", Name: "Romanian", Codes: []string{"ro"}, Regions: []string{"RO"}},
		{ID: "el", Name: "Greek", Codes: []string{"el"}, Regions: []string{"GR"}, Shared: []string{"CY"}},
		{ID: "bg", Name: "Bulgarian", Codes: []string{"bg"}, Regions: []string{"BG"}},
		{ID: "hr", Name: "Croatian", Codes: []string{"hr"}, Regions: []string{"HR"}, Shared: []string{"YUCS", "XYU"}},
		{ID: "sr", Name: "Serbian", Codes: []string{"sr"}, Regions: []string{"RS"}, Shared: []string{"YUCS", "XYU"}},
		{ID: "sl", Name: "Slovenian", Codes: []string{"sl"}, Regions: []string{"SI"}},
		{ID: "tr", Name: "Turkish", Codes: []string{"tr"}, Regions: []string{"TR"}},
		{ID: "ru", Name: "Russian", Codes: []string{"ru"}, Regions: []string{"RU"}, Shared: []string{"SUHH", "UA", "BY", "KZ"}},
		{ID: "uk", Name: "Ukrainian", Codes: []string{"uk"}, Regions: []string{"UA"}},
		{ID: "he", Name: "Hebrew", Codes: []string{"he"}, Regions: []string{"IL"}},
		{ID: "ar", Name: "Arabic", Codes: []string{"ar"}, Regions: []string{"EG", "SA", "AE", "JO", "KW", "QA"}, Shared: []string{"LB", "MA", "DZ", "TN"}},
		{ID: "hi", Name: "Hindi", Codes: []string{"hi"}, Shared: []string{"IN"}},
		{ID: "th", Name: "Thai", Codes: []string{"th"}, Regions: []string{"TH"}},
		{ID: "vi", Name: "Vietnamese", Codes: []string{"vi"}, Regions: []string{"VN"}},
		{ID: "id", Name: "Indonesian", Codes: []string{"id"}, Regions: []string{"ID"}},
		{ID: "ja", Name: "Japanese", Codes: []string{"ja"}, Regions: []string{"JP"}},
		{ID: "ko", Name: "Korean", Codes: []string{"ko"}, Regions: []string{"KR"}},
		{ID: "zh", Name: "Chinese", Codes: []string{"zh", "cmn", "yue"}, Regions: []string{"CN", "TW", "HK"}, Shared: []string{"SG"}, Choices: regions("CN", "TW", "HK")},
	}
}

func LanguageByID(id string) (Language, bool) {
	for _, language := range Languages() {
		if language.ID == id {
			return language, true
		}
	}
	return Language{}, false
}

// AliasOptions selects alternative titles. The zero value disables them, which
// is the behaviour of every catalogue made before this option existed.
type AliasOptions struct {
	Language string `json:"language,omitempty"`
	Region   string `json:"region,omitempty"`
}

func (o AliasOptions) Enabled() bool { return o.Language != "" }

// Validate rejects unknown languages and regions that are not offered for the
// chosen language. A region without a language is meaningless.
func (o AliasOptions) Validate() error {
	if o.Language == "" {
		if o.Region != "" {
			return errAliasChoice
		}
		return nil
	}
	language, ok := LanguageByID(o.Language)
	if !ok {
		return errAliasChoice
	}
	if o.Region != "" && !slices.ContainsFunc(language.Choices, func(r Region) bool { return r.ID == o.Region }) {
		return errAliasChoice
	}
	return nil
}

// accepts applies the language/region rules described on Language. IMDb uses
// \N for a missing value.
func (l Language) accepts(region, language, preferredRegion string) bool {
	if language != `\N` && language != "" && !slices.Contains(l.Codes, language) {
		return false
	}
	explicit := language != `\N` && language != ""
	if preferredRegion != "" {
		if region != preferredRegion {
			return false
		}
		return explicit || slices.Contains(l.Regions, region)
	}
	if explicit {
		return true
	}
	return slices.Contains(l.Regions, region)
}
