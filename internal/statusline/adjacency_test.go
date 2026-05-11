package statusline

import (
	"strings"
	"testing"

	"github.com/Veraticus/cc-tools/internal/aliases"
)

// TestAdjacencyInvariant_R9 enforces R9: across all combinations of
// present/absent right-side chips and all env states, no two adjacent
// chips share the same exact color.
//
// Chain (this task): devspace | host | git | aws | k8s.
// gcloud is not yet in the chain; a follow-up task extends this test.
func TestAdjacencyInvariant_R9(t *testing.T) {
	// Inputs that drive AWS env classification.
	awsByEnv := map[aliases.Env]string{
		aliases.EnvUnknown: "",
		aliases.EnvProd:    "acme-prod-admin",
		aliases.EnvStaging: "acme-staging-admin",
		aliases.EnvDev:     "acme-dev-admin",
	}
	// Inputs that drive k8s env classification.
	k8sByEnv := map[aliases.Env]string{
		aliases.EnvUnknown: "",
		aliases.EnvProd:    "prod-cluster",
		aliases.EnvStaging: "staging-cluster",
		aliases.EnvDev:     "dev-cluster",
	}

	envs := []aliases.Env{
		aliases.EnvUnknown,
		aliases.EnvProd,
		aliases.EnvStaging,
		aliases.EnvDev,
	}

	for _, devspacePresent := range []bool{false, true} {
		for _, gitPresent := range []bool{false, true} {
			for _, awsEnv := range envs {
				for _, k8sEnv := range envs {
					name := chainName(devspacePresent, gitPresent, awsEnv, k8sEnv)
					t.Run(name, func(t *testing.T) {
						s := newTestStatusline(t, newTestResolver(t, ""))
						data := &CachedData{
							Hostname:  "ultraviolet",
							TermWidth: 240,
						}
						if devspacePresent {
							data.Devspace = "mercury"
						}
						if gitPresent {
							data.GitBranch = "main"
						}
						if k8sEnv != aliases.EnvUnknown || k8sByEnv[k8sEnv] != "" {
							data.K8sContext = k8sByEnv[k8sEnv]
						}

						comps := s.collectRightComponents(
							data,
							awsByEnv[awsEnv],
							componentMaxLengths{hostname: 20, branch: 25, aws: 20, k8s: 20, devspace: 15},
						)
						assertNoAdjacentSameColor(t, comps)
					})
				}
			}
		}
	}
}

func assertNoAdjacentSameColor(t *testing.T, comps []Component) {
	t.Helper()
	for i := 1; i < len(comps); i++ {
		if comps[i].Color == comps[i-1].Color {
			t.Errorf(
				"adjacent chips share color %q at positions %d-%d; chain colors=%s",
				comps[i].Color, i-1, i, colorChain(comps),
			)
		}
	}
}

func colorChain(comps []Component) string {
	parts := make([]string, len(comps))
	for i, c := range comps {
		parts[i] = c.Color
	}
	return "[" + strings.Join(parts, " | ") + "]"
}

func chainName(devspace, git bool, awsEnv, k8sEnv aliases.Env) string {
	var sb strings.Builder
	if devspace {
		sb.WriteString("dev_")
	}
	sb.WriteString("host_")
	if git {
		sb.WriteString("git_")
	}
	sb.WriteString("aws=")
	sb.WriteString(string(awsEnv))
	sb.WriteString("_k8s=")
	sb.WriteString(string(k8sEnv))
	return sb.String()
}
