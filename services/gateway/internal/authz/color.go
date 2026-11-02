package authz

// Palette matches presence-service PickColorForUser (legacy pickColorForUser).
var palette = []string{
	"#e6194b", "#3cb44b", "#ffe119", "#4363d8", "#f58231", "#911eb4", "#46f0f0",
	"#f032e6", "#bcf60c", "#fabebe", "#008080", "#e6beff", "#9a6324", "#fffac8",
	"#800000", "#aaffc3", "#808000", "#ffd8b1", "#000075", "#808080",
}

// Color returns the deterministic cursor color for userID.
func Color(userID string) string {
	if userID == "" {
		return palette[0]
	}
	sum := 0
	for i := 0; i < len(userID); i++ {
		sum += int(userID[i])
	}
	return palette[sum%len(palette)]
}
