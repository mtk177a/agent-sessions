package codex

type storageProfile uint8

const (
	profileUnsupported storageProfile = iota
	profileLegacy
	profileCanonicalizedLegacyPaginated
	profileNativePaginated
)

type versionCompatibility struct {
	paginated storageProfile
	legacy    bool
}

var codexCompatibility = map[string]versionCompatibility{
	"0.92.0":            {paginated: profileCanonicalizedLegacyPaginated},
	"0.94.0":            {paginated: profileCanonicalizedLegacyPaginated},
	"0.94.0-alpha.10":   {paginated: profileCanonicalizedLegacyPaginated},
	"0.95.0-alpha.3":    {paginated: profileCanonicalizedLegacyPaginated},
	"0.98.0":            {paginated: profileCanonicalizedLegacyPaginated},
	"0.99.0-alpha.5":    {paginated: profileCanonicalizedLegacyPaginated},
	"0.101.0":           {paginated: profileCanonicalizedLegacyPaginated},
	"0.117.0":           {paginated: profileCanonicalizedLegacyPaginated},
	"0.118.0":           {paginated: profileCanonicalizedLegacyPaginated},
	"0.139.0":           {paginated: profileCanonicalizedLegacyPaginated},
	"0.142.5":           {paginated: profileCanonicalizedLegacyPaginated},
	"0.144.2":           {paginated: profileCanonicalizedLegacyPaginated},
	"0.144.5":           {paginated: profileNativePaginated},
	"0.145.0-alpha.27":  {paginated: profileNativePaginated},
	"0.145.0-alpha.30":  {paginated: profileNativePaginated},
	"0.146.0-alpha.9.2": {paginated: profileNativePaginated},
	"0.147.0":           {paginated: profileNativePaginated},
	"0.147.0-alpha.6.5": {paginated: profileNativePaginated},
	"0.148.0-alpha.9":   {paginated: profileNativePaginated},
	"0.149.0-alpha.4":   {paginated: profileNativePaginated},
	"0.149.0-alpha.4.1": {paginated: profileNativePaginated},
	"0.149.0-alpha.4.3": {paginated: profileNativePaginated},
	"0.149.1":           {paginated: profileNativePaginated, legacy: true},
	"0.150.0-alpha.8":   {paginated: profileNativePaginated},
	"0.152.0":           {paginated: profileNativePaginated, legacy: true},
	"0.153.0":           {paginated: profileNativePaginated, legacy: true},
	"0.153.0-alpha.5":   {paginated: profileNativePaginated},
	"0.153.3":           {paginated: profileNativePaginated, legacy: true},
	"0.153.4":           {paginated: profileNativePaginated, legacy: true},
	"0.154.0":           {paginated: profileNativePaginated, legacy: true},
	"0.154.0-alpha.6.2": {paginated: profileNativePaginated, legacy: true},
	"0.155.0-alpha.9.2": {paginated: profileNativePaginated, legacy: true},
}

func compatibilityProfile(version, historyMode string) storageProfile {
	compatibility, ok := codexCompatibility[version]
	if !ok {
		return profileUnsupported
	}
	switch historyMode {
	case "paginated":
		return compatibility.paginated
	case "legacy":
		if compatibility.legacy {
			return profileLegacy
		}
	}
	return profileUnsupported
}

func supportsInteractionTime(version, historyMode string) bool {
	return compatibilityProfile(version, historyMode) != profileUnsupported
}

func supportsTokenUsageRecord(value string) bool {
	compatibility, ok := codexCompatibility[value]
	return ok && compatibility.paginated == profileNativePaginated && value != supportedVersion
}
