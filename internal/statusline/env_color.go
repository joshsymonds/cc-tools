package statusline

import "github.com/Veraticus/cc-tools/internal/aliases"

// awsBgColor returns the chip background color name for an AWS profile's env.
// Matches epic R4: peach (unknown), red (prod), peach (staging), green (dev).
// Staging shares peach with unknown — the chip text still distinguishes them
// and adjacency invariants tolerate the reuse.
func awsBgColor(env aliases.Env) string {
	switch env {
	case aliases.EnvProd:
		return "red"
	case aliases.EnvDev:
		return "green"
	case aliases.EnvStaging, aliases.EnvUnknown:
		return "peach"
	default:
		return "peach"
	}
}

// gcloudBgColor returns the chip background color name for a gcloud project's env.
// Matches epic R3: lavender (unknown), pink (prod), mauve (staging), sapphire (dev).
// These colors are reused from other chip positions but the adjacency invariant
// (R9) keeps them safe: gcloud sits between aws and k8s, neither of which uses
// these values.
func gcloudBgColor(env aliases.Env) string {
	switch env {
	case aliases.EnvProd:
		return "pink"
	case aliases.EnvStaging:
		return "mauve"
	case aliases.EnvDev:
		return "sapphire"
	case aliases.EnvUnknown:
		return "lavender"
	default:
		return "lavender"
	}
}

// k8sBgColor returns the chip background color name for a k8s context's env.
// Matches epic R4: teal (unknown), maroon (prod), yellow (staging), teal (dev).
// Dev shares teal with unknown by design.
func k8sBgColor(env aliases.Env) string {
	switch env {
	case aliases.EnvProd:
		return "maroon"
	case aliases.EnvStaging:
		return "yellow"
	case aliases.EnvDev, aliases.EnvUnknown:
		return "teal"
	default:
		return "teal"
	}
}
