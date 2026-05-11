package statusline

import (
	"strings"
	"testing"

	"github.com/Veraticus/cc-tools/internal/aliases"
)

func newCloudsDeps(t *testing.T, env map[string]string, files map[string][]byte, resolver *aliases.Resolver) *Dependencies {
	t.Helper()
	envReader := NewMockEnvReader()
	for k, v := range env {
		envReader.vars[k] = v
	}
	fileReader := NewMockFileReader()
	for path, data := range files {
		fileReader.files[path] = data
	}
	return &Dependencies{
		FileReader:    fileReader,
		CommandRunner: NewMockCommandRunner(),
		EnvReader:     envReader,
		TerminalWidth: &MockTerminalWidth{},
		Resolver:      resolver,
	}
}

func TestRenderClouds_NoChipsClosingCurveOnly(t *testing.T) {
	r, _ := aliases.NewResolver("")
	deps := newCloudsDeps(t, nil, nil, r)
	out := RenderClouds(deps)
	if !strings.Contains(out, RightCurve) {
		t.Errorf("empty chain should still emit closing right curve, got %q", out)
	}
	// Sky fg means closing back to git's color.
	if !strings.Contains(out, (CatppuccinMocha{}).SkyFG()) {
		t.Errorf("closing curve should be sky fg (git color); got escapes %q", out)
	}
}

func TestRenderClouds_AwsOnly(t *testing.T) {
	r, _ := aliases.NewResolver("")
	deps := newCloudsDeps(t, map[string]string{"AWS_PROFILE": "acme-prod-admin"}, nil, r)
	out := RenderClouds(deps)
	if !strings.Contains(out, "acme-prod-admin") {
		t.Errorf("output should contain AWS profile label, got %q", out)
	}
	if !strings.Contains(out, (CatppuccinMocha{}).RedBG()) {
		t.Errorf("prod AWS should render with red bg, got %q", out)
	}
}

func TestRenderClouds_AllThreeChipsOrdered(t *testing.T) {
	r, _ := aliases.NewResolver("")
	// kubectl reads from KUBECONFIG; provide a minimal kubeconfig file via the
	// mock file reader at a known path and set KUBECONFIG to point there.
	kubeconfig := []byte("current-context: prod-cluster\n")
	deps := newCloudsDeps(t,
		map[string]string{
			"AWS_PROFILE": "dev-profile",
			"KUBECONFIG":  "/fake/kubeconfig",
		},
		map[string][]byte{"/fake/kubeconfig": kubeconfig},
		r,
	)
	// Gcloud is read from filesystem. Provide files via mock.
	home := "/fake/home"
	deps.EnvReader.(*MockEnvReader).vars["HOME"] = home
	mfr := deps.FileReader.(*MockFileReader)
	mfr.files[home+"/.config/gcloud/active_config"] = []byte("default")
	mfr.files[home+"/.config/gcloud/configurations/config_default"] = []byte("[core]\nproject = stage-project\n")

	out := RenderClouds(deps)

	// Expected order in output: aws → gcloud → k8s.
	awsIdx := strings.Index(out, "dev-profile")
	gcloudIdx := strings.Index(out, "stage-project")
	k8sIdx := strings.Index(out, "prod-cluster")

	if awsIdx < 0 {
		t.Fatalf("output missing aws label: %q", out)
	}
	if gcloudIdx < 0 {
		t.Fatalf("output missing gcloud label: %q", out)
	}
	if k8sIdx < 0 {
		t.Fatalf("output missing k8s label: %q", out)
	}
	if !(awsIdx < gcloudIdx && gcloudIdx < k8sIdx) {
		t.Errorf("expected order aws < gcloud < k8s in output: aws=%d gcloud=%d k8s=%d",
			awsIdx, gcloudIdx, k8sIdx)
	}
}

func TestRenderClouds_LeadingChevronFromSky(t *testing.T) {
	r, _ := aliases.NewResolver("")
	deps := newCloudsDeps(t, map[string]string{"AWS_PROFILE": "personal"}, nil, r)
	out := RenderClouds(deps)
	// Leading chevron should have sky FG (transitioning from git's color).
	skyFG := (CatppuccinMocha{}).SkyFG()
	if !strings.Contains(out, skyFG) {
		t.Errorf("leading chevron should use sky fg (git → aws transition); got %q", out)
	}
}

func TestRenderClouds_ChipUsesAliasResolution(t *testing.T) {
	r, _ := aliases.NewResolver("")
	// Use an EKS ARN; resolver should strip it.
	kubeconfig := []byte("current-context: arn:aws:eks:us-east-1:123456789012:cluster/widgets-prod\n")
	deps := newCloudsDeps(t,
		map[string]string{"KUBECONFIG": "/fake/kc"},
		map[string][]byte{"/fake/kc": kubeconfig},
		r,
	)
	out := RenderClouds(deps)
	if strings.Contains(out, "arn:aws:eks") {
		t.Errorf("ARN should be stripped, got %q", out)
	}
	if !strings.Contains(out, "widgets-prod") {
		t.Errorf("output should contain stripped cluster name, got %q", out)
	}
}
