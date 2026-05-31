package brew

// Kind distinguishes the two things Homebrew installs. Most brew subcommands
// branch on it (--formula vs --cask), so callers pass it explicitly rather than
// us guessing from a name.
type Kind int

const (
	KindFormula Kind = iota
	KindCask
)

func (k Kind) String() string {
	if k == KindCask {
		return "cask"
	}
	return "formula"
}

// Formula projects the subset of `brew info --json=v2` formula fields the UI
// renders. The full payload has ~60 fields; modelling only what we show keeps
// the type honest about what the app actually depends on.
type Formula struct {
	Name              string         `json:"name"`
	FullName          string         `json:"full_name"`
	Tap               string         `json:"tap"`
	Desc              string         `json:"desc"`
	License           string         `json:"license"`
	Homepage          string         `json:"homepage"`
	Versions          Versions       `json:"versions"`
	Installed         []InstalledKeg `json:"installed"`
	Outdated          bool           `json:"outdated"`
	Pinned            bool           `json:"pinned"`
	KegOnly           bool           `json:"keg_only"`
	Deprecated        bool           `json:"deprecated"`
	Caveats           string         `json:"caveats"`
	Dependencies      []string       `json:"dependencies"`
	BuildDependencies []string       `json:"build_dependencies"`
}

// Versions mirrors the formula "versions" object; Stable is what we display.
type Versions struct {
	Stable string `json:"stable"`
	Head   string `json:"head"`
	Bottle bool   `json:"bottle"`
}

// InstalledKeg is one on-disk installation of a formula. A formula can have
// several (e.g. versioned kegs); the last entry is the active version.
type InstalledKeg struct {
	Version            string `json:"version"`
	InstalledOnRequest bool   `json:"installed_on_request"`
}

// InstalledVersion returns the active installed version, or "" if not installed.
func (f Formula) InstalledVersion() string {
	if len(f.Installed) == 0 {
		return ""
	}
	return f.Installed[len(f.Installed)-1].Version
}

// IsInstalled reports whether the formula has at least one keg on disk.
func (f Formula) IsInstalled() bool { return len(f.Installed) > 0 }

// Cask projects the cask fields the UI renders. Note the schema quirks: "name"
// is an array of human-readable names and "installed" is a bare version string
// (empty when not installed), unlike a formula's structured "installed" array.
type Cask struct {
	Token       string   `json:"token"`
	FullToken   string   `json:"full_token"`
	Tap         string   `json:"tap"`
	Names       []string `json:"name"`
	Desc        string   `json:"desc"`
	Homepage    string   `json:"homepage"`
	Version     string   `json:"version"`
	Installed   string   `json:"installed"`
	Outdated    bool     `json:"outdated"`
	Deprecated  bool     `json:"deprecated"`
	AutoUpdates bool     `json:"auto_updates"`
	Caveats     string   `json:"caveats"`
}

// IsInstalled reports whether a version is recorded on disk.
func (c Cask) IsInstalled() bool { return c.Installed != "" }

// DisplayName prefers the cask's human-readable name, falling back to its token.
func (c Cask) DisplayName() string {
	if len(c.Names) > 0 && c.Names[0] != "" {
		return c.Names[0]
	}
	return c.Token
}

// Tap projects `brew tap-info --json` fields. The formula/cask name slices are
// kept so the UI can show counts without a second command per tap.
type Tap struct {
	Name         string   `json:"name"`
	Official     bool     `json:"official"`
	Installed    bool     `json:"installed"`
	Remote       string   `json:"remote"`
	FormulaNames []string `json:"formula_names"`
	CaskTokens   []string `json:"cask_tokens"`
}

func (t Tap) FormulaCount() int { return len(t.FormulaNames) }
func (t Tap) CaskCount() int    { return len(t.CaskTokens) }

// OutdatedReport is `brew outdated --json=v2`. Formulae and casks share the
// same entry shape, so OutdatedPackage covers both.
type OutdatedReport struct {
	Formulae []OutdatedPackage `json:"formulae"`
	Casks    []OutdatedPackage `json:"casks"`
}

// OutdatedPackage names a package and the version it would move to. Tapped
// packages appear with their fully-qualified name (e.g. "hashicorp/tap/x").
type OutdatedPackage struct {
	Name              string   `json:"name"`
	InstalledVersions []string `json:"installed_versions"`
	CurrentVersion    string   `json:"current_version"`
	Pinned            bool     `json:"pinned"`
}

// InstalledVersion returns the first recorded installed version, or "".
func (o OutdatedPackage) InstalledVersion() string {
	if len(o.InstalledVersions) == 0 {
		return ""
	}
	return o.InstalledVersions[0]
}

// Total counts every upgradeable package across both kinds.
func (r OutdatedReport) Total() int { return len(r.Formulae) + len(r.Casks) }

// DoctorReport is the parsed result of `brew doctor`, which emits free-form
// text (not JSON) and exits non-zero when warnings exist.
type DoctorReport struct {
	OK       bool
	Warnings []Warning
	Raw      string
}

// Warning is one `brew doctor` warning block: a title line plus its detail lines.
type Warning struct {
	Title   string
	Details []string
}
