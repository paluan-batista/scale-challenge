package bootstrap

import "testing"

func TestRequiredEnvironmentRejectsMissingValueWithoutLeakingValue(t *testing.T) {
	t.Setenv("SCALE_TEST_REQUIRED", "")
	if err := RequiredEnvironment("SCALE_TEST_REQUIRED"); err == nil {
		t.Fatal("RequiredEnvironment() error = nil, want missing-variable error")
	}
}

func TestRequiredEnvironmentAcceptsConfiguredValue(t *testing.T) {
	t.Setenv("SCALE_TEST_REQUIRED", "configured")
	if err := RequiredEnvironment("SCALE_TEST_REQUIRED"); err != nil {
		t.Fatalf("RequiredEnvironment() error = %v", err)
	}
}
