package pricing

import "strings"

// PricingRules - ported verbatim from src/utils/islandPricing.json
var PricingRules = map[string]float64{
	"Pulau Jawa":       1.01,
	"Pulau Sumatra":    1,
	"Pulau Kalimantan": 1.035,
	"Pulau Sulawesi":   1.036,
	"Pulau Papua":      1.1,
	"nonCustomer":      1.08182,
}

// ProvinceIslandEntry is one entry of the province->island mapping.
// Kept as an ordered slice (not a map) because getIslandFromProvince's
// substring-match fallback depends on iteration (insertion) order, matching
// JS's Object.entries ordering guarantee.
type ProvinceIslandEntry struct {
	Province string
	Island   string
}

// ProvinceToIslandMapping - ported verbatim from src/utils/provinceMapping.js
var ProvinceToIslandMapping = []ProvinceIslandEntry{
	{"Jakarta", "Pulau Jawa"},
	{"DKI Jakarta", "Pulau Jawa"},
	{"Dki Jakarta", "Pulau Jawa"},
	{"Daerah Khusus Ibukota Jakarta", "Pulau Jawa"},
	{"Jawa Barat", "Pulau Jawa"},
	{"Jabar", "Pulau Jawa"},
	{"Jawa Tengah", "Pulau Jawa"},
	{"Jateng", "Pulau Jawa"},
	{"Jawa Timur", "Pulau Jawa"},
	{"Jatim", "Pulau Jawa"},
	{"Banten", "Pulau Jawa"},
	{"Yogyakarta", "Pulau Jawa"},
	{"DI Yogyakarta", "Pulau Jawa"},
	{"Daerah Istimewa Yogyakarta", "Pulau Jawa"},
	{"DIY", "Pulau Jawa"},
	{"Jogja", "Pulau Jawa"},
	{"Jogjakarta", "Pulau Jawa"},
	{"Bandung", "Pulau Jawa"},
	{"Semarang", "Pulau Jawa"},
	{"Surabaya", "Pulau Jawa"},
	{"Solo", "Pulau Jawa"},
	{"Surakarta", "Pulau Jawa"},
	{"Malang", "Pulau Jawa"},
	{"Teluk Betung Utara", "Pulau Jawa"},
	{"Aceh", "Pulau Sumatra"},
	{"Nanggroe Aceh Darussalam", "Pulau Sumatra"},
	{"NAD", "Pulau Sumatra"},
	{"Sumatera Utara", "Pulau Sumatra"},
	{"Sumut", "Pulau Sumatra"},
	{"SUMATRA UTARA", "Pulau Sumatra"},
	{"SUMATERA UTARA", "Pulau Sumatra"},
	{"Sumatra Utara", "Pulau Sumatra"},
	{"Sumatera Barat", "Pulau Sumatra"},
	{"Sumbar", "Pulau Sumatra"},
	{"Sumatra Barat", "Pulau Sumatra"},
	{"Sumatera Selatan", "Pulau Sumatra"},
	{"Sumsel", "Pulau Sumatra"},
	{"Sumatra Selatan", "Pulau Sumatra"},
	{"Riau", "Pulau Sumatra"},
	{"Kepulauan Riau", "Pulau Sumatra"},
	{"Kepri", "Pulau Sumatra"},
	{"Jambi", "Pulau Sumatra"},
	{"Bengkulu", "Pulau Sumatra"},
	{"Lampung", "Pulau Sumatra"},
	{"Bandar Lampung", "Pulau Sumatra"},
	{"Bangka Belitung", "Pulau Sumatra"},
	{"Kepulauan Bangka Belitung", "Pulau Sumatra"},
	{"Babel", "Pulau Sumatra"},
	{"Bangka", "Pulau Sumatra"},
	{"Belitung", "Pulau Sumatra"},
	{"Medan", "Pulau Sumatra"},
	{"Padang", "Pulau Sumatra"},
	{"Palembang", "Pulau Sumatra"},
	{"Pekanbaru", "Pulau Sumatra"},
	{"Pekan Baru", "Pulau Sumatra"},
	{"Batam", "Pulau Sumatra"},
	{"Kalimantan Barat", "Pulau Kalimantan"},
	{"Kalbar", "Pulau Kalimantan"},
	{"Kalimantan Tengah", "Pulau Kalimantan"},
	{"Kalteng", "Pulau Kalimantan"},
	{"Kalimantan Selatan", "Pulau Kalimantan"},
	{"Kalsel", "Pulau Kalimantan"},
	{"Kalimantan Timur", "Pulau Kalimantan"},
	{"Kaltim", "Pulau Kalimantan"},
	{"Kalimantan Utara", "Pulau Kalimantan"},
	{"Kaltara", "Pulau Kalimantan"},
	{"Kalimantan Tegah", "Pulau Kalimantan"},
	{"KALIMANTAN", "Pulau Kalimantan"},
	{"Pontianak", "Pulau Kalimantan"},
	{"Palangkaraya", "Pulau Kalimantan"},
	{"Palangka Raya", "Pulau Kalimantan"},
	{"Banjarmasin", "Pulau Kalimantan"},
	{"Samarinda", "Pulau Kalimantan"},
	{"Balikpapan", "Pulau Kalimantan"},
	{"Tanjung Selor", "Pulau Kalimantan"},
	{"Sulawesi Utara", "Pulau Sulawesi"},
	{"Sulut", "Pulau Sulawesi"},
	{"Sulawesi Tengah", "Pulau Sulawesi"},
	{"Sulteng", "Pulau Sulawesi"},
	{"Sulawesi Selatan", "Pulau Sulawesi"},
	{"Sulsel", "Pulau Sulawesi"},
	{"Sulawesi Tenggara", "Pulau Sulawesi"},
	{"Sultra", "Pulau Sulawesi"},
	{"Sulawesi Barat", "Pulau Sulawesi"},
	{"Sulbar", "Pulau Sulawesi"},
	{"Sulawesi", "Pulau Sulawesi"},
	{"Gorontalo", "Pulau Sulawesi"},
	{"Manado", "Pulau Sulawesi"},
	{"Palu", "Pulau Sulawesi"},
	{"Makassar", "Pulau Sulawesi"},
	{"Makasar", "Pulau Sulawesi"},
	{"Ujung Pandang", "Pulau Sulawesi"},
	{"Kendari", "Pulau Sulawesi"},
	{"Mamuju", "Pulau Sulawesi"},
	{"Papua", "Pulau Papua"},
	{"Papua Barat", "Pulau Papua"},
	{"Papua Tengah", "Pulau Papua"},
	{"Papua Pegunungan", "Pulau Papua"},
	{"Papua Selatan", "Pulau Papua"},
	{"Papua Barat Daya", "Pulau Papua"},
	{"Irian Jaya", "Pulau Papua"},
	{"Jayapura", "Pulau Papua"},
	{"Sorong", "Pulau Papua"},
	{"Manokwari", "Pulau Papua"},
	{"Nabire", "Pulau Papua"},
	{"Merauke", "Pulau Papua"},
	{"Maluku", "Pulau Maluku"},
	{"Maluku Utara", "Pulau Maluku"},
	{"Ambon", "Pulau Maluku"},
	{"Ternate", "Pulau Maluku"},
	{"Tidore", "Pulau Maluku"},
	{"Bali", "Pulau Jawa"},
	{"Nusa Tenggara Barat", "Pulau Jawa"},
	{"NTB", "Pulau Jawa"},
	{"Nusa Tenggara Timur", "Pulau Jawa"},
	{"NTT", "Pulau Jawa"},
	{"Denpasar", "Pulau Jawa"},
	{"Mataram", "Pulau Jawa"},
	{"Kupang", "Pulau Jawa"},
	{"Lombok", "Pulau Jawa"},
}

// GetIslandFromProvince - ported verbatim from provinceMapping.js
// getIslandFromProvince(): exact match first, then substring match
// (either direction), defaulting to "Pulau Kalimantan".
func GetIslandFromProvince(province string) string {
	if province == "" {
		return "Pulau Kalimantan"
	}

	normalizedProvince := strings.ToLower(strings.TrimSpace(province))

	for _, entry := range ProvinceToIslandMapping {
		if normalizedProvince == strings.ToLower(entry.Province) {
			return entry.Island
		}
	}

	for _, entry := range ProvinceToIslandMapping {
		normalizedKey := strings.ToLower(entry.Province)
		if strings.Contains(normalizedProvince, normalizedKey) || strings.Contains(normalizedKey, normalizedProvince) {
			return entry.Island
		}
	}

	return "Pulau Kalimantan"
}
