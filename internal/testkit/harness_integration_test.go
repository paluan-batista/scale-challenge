//go:build integration

package testkit

import "testing"

func TestHarnessProvidesIsolatedPostgresAndRedis(t *testing.T) {
	harness := New(t, WithPostgres(), WithRedis())
	if harness.DB == nil || harness.Redis == nil {
		t.Fatal("integration harness did not provide both dependencies")
	}
	if err := harness.Redis.Set(harness.Context, harness.Fixtures.Name("key"), "value", 0).Err(); err != nil {
		t.Fatalf("write isolated Redis fixture: %v", err)
	}
	harness.AssertNoBusinessMutation(t)
}
