package aliases

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTOML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "aliases.toml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func mustResolver(t *testing.T, content string) *Resolver {
	t.Helper()
	r, err := NewResolver(writeTOML(t, content))
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r
}

func TestResolve_HostExactMatch(t *testing.T) {
	r := mustResolver(t, `
[hosts.ultraviolet]
label = "uv"
`)
	label, env := r.Resolve(KindHost, "ultraviolet")
	if label != "uv" || env != EnvUnknown {
		t.Fatalf("got %q/%s, want uv/unknown", label, env)
	}
}

func TestResolve_HostFallback(t *testing.T) {
	r := mustResolver(t, "")
	label, env := r.Resolve(KindHost, "blackbox")
	if label != "blackbox" || env != EnvUnknown {
		t.Fatalf("got %q/%s, want blackbox/unknown", label, env)
	}
}

func TestResolve_K8sARNStrip(t *testing.T) {
	r := mustResolver(t, "")
	label, env := r.Resolve(KindK8s, "arn:aws:eks:us-east-1:123456789012:cluster/prod-eks")
	if label != "prod-eks" {
		t.Fatalf("label: got %q, want prod-eks", label)
	}
	if env != EnvProd {
		t.Fatalf("env: got %s, want prod", env)
	}
}

func TestResolve_K8sGKEStrip(t *testing.T) {
	r := mustResolver(t, "")
	label, env := r.Resolve(KindK8s, "gke_my-project_us-central1_my-cluster")
	if label != "my-cluster" || env != EnvUnknown {
		t.Fatalf("got %q/%s, want my-cluster/unknown", label, env)
	}
}

func TestResolve_K8sConnectGateway(t *testing.T) {
	r := mustResolver(t, "")
	raw := "connectgateway_klover-loan-application_global_core-production-us-east1"
	label, env := r.Resolve(KindK8s, raw)
	if label != "core-production-us-east1" {
		t.Fatalf("label: got %q, want core-production-us-east1", label)
	}
	if env != EnvProd {
		t.Fatalf("env: got %s, want prod (matched 'production')", env)
	}
}

func TestResolve_K8sExplicitAliasOverridesStrip(t *testing.T) {
	r := mustResolver(t, `
[k8s."connectgateway_klover-loan-application_global_core-production-us-east1"]
label = "klover prod"
env = "prod"
`)
	label, env := r.Resolve(KindK8s, "connectgateway_klover-loan-application_global_core-production-us-east1")
	if label != "klover prod" || env != EnvProd {
		t.Fatalf("got %q/%s, want 'klover prod'/prod", label, env)
	}
}

func TestResolve_AWSExplicitEnv(t *testing.T) {
	r := mustResolver(t, `
[aws."weird-name"]
label = "wn"
env = "prod"
`)
	label, env := r.Resolve(KindAWS, "weird-name")
	if label != "wn" || env != EnvProd {
		t.Fatalf("got %q/%s, want wn/prod", label, env)
	}
}

func TestResolve_AWSPatternEnv(t *testing.T) {
	r := mustResolver(t, "")
	label, env := r.Resolve(KindAWS, "acme-staging-admin")
	if label != "acme-staging-admin" || env != EnvStaging {
		t.Fatalf("got %q/%s, want acme-staging-admin/staging", label, env)
	}
}

func TestResolve_GcloudProject(t *testing.T) {
	r := mustResolver(t, `
[gcloud."klover-loan-application"]
label = "klover"
env = "prod"
`)
	label, env := r.Resolve(KindGcloud, "klover-loan-application")
	if label != "klover" || env != EnvProd {
		t.Fatalf("got %q/%s, want klover/prod", label, env)
	}
}

func TestResolve_MissingFile(t *testing.T) {
	r, err := NewResolver("/nonexistent/path/aliases.toml")
	if err != nil {
		t.Fatalf("NewResolver should not error on missing file: %v", err)
	}
	label, env := r.Resolve(KindHost, "ultraviolet")
	if label != "ultraviolet" || env != EnvUnknown {
		t.Fatalf("got %q/%s, want ultraviolet/unknown", label, env)
	}
	_, env2 := r.Resolve(KindAWS, "acme-prod-admin")
	if env2 != EnvProd {
		t.Fatalf("classify with default patterns: got %s, want prod", env2)
	}
}

func TestResolve_EmptyRaw(t *testing.T) {
	r := mustResolver(t, "")
	label, env := r.Resolve(KindHost, "")
	if label != "" || env != EnvUnknown {
		t.Fatalf("got %q/%s, want empty/unknown", label, env)
	}
}

func TestResolve_EmptyEnvPatternsFallsBackToDefaults(t *testing.T) {
	// An explicit empty `[env_patterns]` table — no keys defined —
	// silently falls back to the built-in defaults. This pins the
	// current behavior: empty-but-present means "I didn't override,"
	// not "I want zero patterns." Override that means "zero patterns"
	// is unsupported by design.
	r := mustResolver(t, `
[env_patterns]
`)
	_, env := r.Resolve(KindAWS, "acme-prod-admin")
	if env != EnvProd {
		t.Errorf("empty [env_patterns] should fall back to default patterns; got %s, want prod", env)
	}
}

func TestResolve_CustomEnvPatterns(t *testing.T) {
	r := mustResolver(t, `
[env_patterns]
prod = ["mainline"]
staging = ["pre"]
dev = ["lab"]
`)
	_, env := r.Resolve(KindAWS, "mainline-account")
	if env != EnvProd {
		t.Fatalf("custom prod pattern: got %s, want prod", env)
	}
	_, env = r.Resolve(KindAWS, "production-account")
	if env != EnvUnknown {
		t.Fatalf("custom patterns should REPLACE defaults: got %s, want unknown", env)
	}
}

func TestResolve_AliasWithoutLabel(t *testing.T) {
	r := mustResolver(t, `
[aws."acme-dev"]
env = "dev"
`)
	label, env := r.Resolve(KindAWS, "acme-dev")
	if label != "acme-dev" || env != EnvDev {
		t.Fatalf("got %q/%s, want acme-dev/dev", label, env)
	}
}

func TestResolve_AliasLabelOnly(t *testing.T) {
	r := mustResolver(t, `
[aws."acme-prod-administrator"]
label = "acme prod"
`)
	label, env := r.Resolve(KindAWS, "acme-prod-administrator")
	if label != "acme prod" {
		t.Fatalf("label: got %q, want 'acme prod'", label)
	}
	if env != EnvProd {
		t.Fatalf("env: classify falls through to pattern match on raw: got %s, want prod", env)
	}
}

func TestResolve_ProdWinsOverStagingWhenBothMatch(t *testing.T) {
	r := mustResolver(t, "")
	_, env := r.Resolve(KindAWS, "prod-staging-mixed")
	if env != EnvProd {
		t.Fatalf("got %s, want prod (prod must win when multiple match)", env)
	}
}

func TestDefaultPath_RespectsOverride(t *testing.T) {
	t.Setenv("STATUSLINE_ALIASES", "/tmp/custom.toml")
	if got := DefaultPath(); got != "/tmp/custom.toml" {
		t.Fatalf("got %q, want /tmp/custom.toml", got)
	}
}

func TestDefaultPath_UsesXDG(t *testing.T) {
	t.Setenv("STATUSLINE_ALIASES", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	if got := DefaultPath(); got != "/tmp/xdg/statusline-aliases/aliases.toml" {
		t.Fatalf("got %q, want /tmp/xdg/statusline-aliases/aliases.toml", got)
	}
}

func TestParseError_Returned(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "broken.toml")
	if err := os.WriteFile(p, []byte("not = valid = toml = at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewResolver(p); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}
