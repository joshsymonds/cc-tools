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
