package environments

import (
	"reflect"
	"testing"

	"github.com/spf13/pflag"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

func TestLoadServices(t *testing.T) {
	// Set environment to unit_testing to use mocks
	t.Setenv("OCM_ENV", "unit_testing")

	// Create minimal configuration for unit test
	cfg := config.NewApplicationConfig()
	cfg.Adapters.Required.Cluster = []string{"validation", "dns", "pullsecret", "hypershift"}
	cfg.Adapters.Required.Nodepool = []string{"validation", "hypershift"}

	env := Environment()
	env.Config = cfg

	err := env.SetEnvironmentDefaults(pflag.CommandLine)
	if err != nil {
		t.Errorf("Unable to add flags for testing environment: %s", err.Error())
		return
	}
	pflag.Parse()
	err = env.Initialize()
	if err != nil {
		t.Errorf("Unable to load testing environment: %s", err.Error())
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
