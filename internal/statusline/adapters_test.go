package statusline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultEnvReader_AwsProfileFromFile(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	if err := os.WriteFile(state, []byte(`{"aws_profile":"foo-prod"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CC_TOOLS_STATE_FILE", state)
	t.Setenv("AWS_PROFILE", "other-profile")

	r := &DefaultEnvReader{}
	if got := r.Get("AWS_PROFILE"); got != "foo-prod" {
		t.Errorf("file should override env: got %q, want foo-prod", got)
	}
}

func TestDefaultEnvReader_AwsProfileEmptyStringFromFile(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	if err := os.WriteFile(state, []byte(`{"aws_profile":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CC_TOOLS_STATE_FILE", state)
	t.Setenv("AWS_PROFILE", "fallback-shouldnt-win")

	r := &DefaultEnvReader{}
	if got := r.Get("AWS_PROFILE"); got != "" {
		t.Errorf("empty string in file should be authoritative: got %q, want empty", got)
	}
}

func TestDefaultEnvReader_AwsProfileFileMissing(t *testing.T) {
	t.Setenv("CC_TOOLS_STATE_FILE", "/nonexistent/state.json")
	t.Setenv("AWS_PROFILE", "from-env")

	r := &DefaultEnvReader{}
	if got := r.Get("AWS_PROFILE"); got != "from-env" {
		t.Errorf("missing file should fall through to env: got %q, want from-env", got)
	}
}

func TestDefaultEnvReader_AwsProfileMalformedFile(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	if err := os.WriteFile(state, []byte(`not json {`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CC_TOOLS_STATE_FILE", state)
	t.Setenv("AWS_PROFILE", "from-env")

	r := &DefaultEnvReader{}
	if got := r.Get("AWS_PROFILE"); got != "from-env" {
		t.Errorf("malformed file should not crash, should fall through: got %q, want from-env", got)
	}
}

func TestDefaultEnvReader_AwsProfileKeyAbsentInFile(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	if err := os.WriteFile(state, []byte(`{"something_else":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CC_TOOLS_STATE_FILE", state)
	t.Setenv("AWS_PROFILE", "from-env")

	r := &DefaultEnvReader{}
	if got := r.Get("AWS_PROFILE"); got != "from-env" {
		t.Errorf("file without aws_profile key should fall through: got %q, want from-env", got)
	}
}

func TestDefaultEnvReader_NonAwsProfileUnchanged(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	if err := os.WriteFile(state, []byte(`{"aws_profile":"foo-prod"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CC_TOOLS_STATE_FILE", state)
	t.Setenv("KUBECONFIG", "/some/path")

	r := &DefaultEnvReader{}
	if got := r.Get("KUBECONFIG"); got != "/some/path" {
		t.Errorf("non-AWS keys should pass through to env: got %q, want /some/path", got)
	}
}
