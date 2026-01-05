package environments

import (
	"os"
	"os/exec"
	"reflect"
	"testing"

	"github.com/spf13/pflag"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

func BenchmarkGetDynos(b *testing.B) {
	b.ReportAllocs()
	fn := func(b *testing.B) {
		cmd := exec.Command("ocm", "get", "/api/hyperfleet/v1/clusters", "params='size=2'")
		_, err := cmd.CombinedOutput()
		if err != nil {
			b.Errorf("ERROR %+v", err)
		}
	}
	for n := 0; n < b.N; n++ {
		fn(b)
	}
}

func TestLoadServices(t *testing.T) {
	// Set required environment variables for testing
	os.Setenv("HYPERFLEET_APP_NAME", "hyperfleet-api-test")
	os.Setenv("HYPERFLEET_OCM_MOCK", "true")
	os.Setenv("HYPERFLEET_OCM_DEBUG", "false")
	os.Setenv("HYPERFLEET_OCM_BASE_URL", "https://api.integration.openshift.com")
	os.Setenv("HYPERFLEET_SERVER_HTTPS_ENABLED", "false")
	os.Setenv("HYPERFLEET_METRICS_HTTPS_ENABLED", "false")
	os.Setenv("HYPERFLEET_AUTH_AUTHZ_ENABLED", "true")

	// Create config
	appConfig := config.NewApplicationConfig()

	// Create viper and configure flags
	v := config.NewCommandConfig()
	appConfig.ConfigureFlags(v, pflag.CommandLine)

	pflag.Parse()

	// Load config
	loadedConfig, err := config.LoadConfig(v, pflag.CommandLine)
	if err != nil {
		t.Errorf("Failed to load configuration: %v", err)
		return
	}

	// Initialize environment with loaded config
	env := Environment()
	err = env.Initialize(loadedConfig)
	if err != nil {
		t.Errorf("Unable to initialize testing environment: %s", err.Error())
		return
	}

	s := reflect.ValueOf(&env.Services).Elem()
	sType := s.Type()

	for i := 0; i < s.NumField(); i++ {
		field := s.Field(i)
		fieldType := sType.Field(i)

		// Skip unexported fields (lowercase first letter)
		if !fieldType.IsExported() {
			continue
		}

		// Only check fields that are function types (service locators)
		if field.Kind() == reflect.Func && field.IsNil() {
			t.Errorf("Service locator %s is nil", fieldType.Name)
		}
	}
}
