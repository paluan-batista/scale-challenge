//go:build acceptance

package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"scale-challenge/internal/simulator"
)

func TestFoundationTopologyAndDeterministicScenarioFixture(t *testing.T) {
	composeFile := filepath.Join("..", "..", "docker-compose.yml")
	contents, err := os.ReadFile(composeFile)
	if err != nil {
		t.Fatal(err)
	}

	for _, required := range []string{"api:", "worker:", "postgres:", "redis:", "simulator:", "test:", "profiles: [\"simulator\"]", "condition: service_healthy"} {
		if !strings.Contains(string(contents), required) {
			t.Errorf("compose topology is missing %q", required)
		}
	}

	scenario, err := simulator.Load(filepath.Join("..", "..", "testdata", "scenarios", "happy-path.json"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := simulator.Sequence(scenario, 42)
	if err != nil {
		t.Fatal(err)
	}
	second, err := simulator.Sequence(scenario, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("seed 42 produced different reading sequences")
	}
}

func TestGherkinSuccessFullEnvironment(t *testing.T) {
	command := exec.Command("docker", "compose", "version")
	if output, err := command.CombinedOutput(); err != nil {
		t.Skipf("Docker Compose v2 prerequisite is unavailable: %s", strings.TrimSpace(string(output)))
	}
	t.Skip("blocked: the migration and seed commands required by this scenario belong to T02 and are not available in T01")
}

func TestGherkinErrorInvalidSimulatorConfigurationFailsWithoutSecret(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "negative seed", args: []string{"--base-url", "http://api:8080", "--scenario", "../../testdata/scenarios/happy-path.json", "--seed", "-1"}, want: "non-negative seed"},
		{name: "zero frequency", args: []string{"--base-url", "http://api:8080", "--scenario", "../../testdata/scenarios/happy-path.json", "--frequency-ms", "0"}, want: "frequency_ms must be greater than zero"},
		{name: "missing base URL", args: []string{"--scenario", "../../testdata/scenarios/happy-path.json"}, want: "base-url, scenario"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			command := exec.Command("go", append([]string{"run", "../../cmd/simulator"}, testCase.args...)...)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatal("invalid simulator command succeeded")
			}
			if strings.Contains(string(output), "scale-local-only") {
				t.Fatalf("simulator output leaked a secret: %s", output)
			}
			if !strings.Contains(string(output), testCase.want) {
				t.Fatalf("simulator output did not identify invalid configuration: %s", output)
			}
		})
	}
}

func TestGherkinErrorServiceMissingRequiredVariableFailsWithoutSecret(t *testing.T) {
	for _, service := range []string{"api", "worker"} {
		t.Run(service, func(t *testing.T) {
			command := exec.Command("go", "run", "../../cmd/"+service)
			command.Env = append(withoutEnvironment("DATABASE_URL", "REDIS_ADDR"), "QA_TEST_SECRET=must-not-appear")
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatal("service started with missing required configuration")
			}
			if strings.Contains(string(output), "must-not-appear") {
				t.Fatalf("service output leaked a secret: %s", output)
			}
			if !strings.Contains(string(output), "DATABASE_URL") {
				t.Fatalf("service output did not identify the missing variable: %s", output)
			}
		})
	}
}

func withoutEnvironment(names ...string) []string {
	blocked := make(map[string]struct{}, len(names))
	for _, name := range names {
		blocked[name] = struct{}{}
	}
	var retained []string
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, excluded := blocked[name]; excluded {
				continue
			}
		}
		retained = append(retained, entry)
	}
	return retained
}
