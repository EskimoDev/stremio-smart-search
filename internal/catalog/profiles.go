package catalog

// Profile bounds both title selection and the ratings retained during a build.
// Limits are ceilings: the source may contain fewer qualifying titles.
type Profile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Movies   int    `json:"movies"`
	Series   int    `json:"series"`
	MinVotes int    `json:"minVotes"`
	// Aliases bounds alternative titles kept for this size (most-voted titles
	// first), so turning them on adds a bounded amount of index memory.
	Aliases int `json:"aliases"`
}

const DefaultProfile = "standard"

func Profiles() []Profile {
	return []Profile{
		{ID: "compact", Name: "Compact", Movies: 25000, Series: 25000, MinVotes: 250, Aliases: 75000},
		{ID: "standard", Name: "Standard", Movies: 100000, Series: 100000, MinVotes: 25, Aliases: 200000},
		{ID: "expanded", Name: "Expanded", Movies: 250000, Series: 250000, MinVotes: 5, Aliases: 200000},
	}
}

func ProfileByID(id string) (Profile, bool) {
	for _, profile := range Profiles() {
		if profile.ID == id {
			return profile, true
		}
	}
	return Profile{}, false
}
