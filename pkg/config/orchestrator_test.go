package config

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestOrchestratorConfigDefaults(t *testing.T) {
	RegisterTestingT(t)

	cfg := NewOrchestratorConfig()

	Expect(cfg.Enabled).To(BeFalse())
	Expect(cfg.PollInterval).To(Equal(5 * time.Second))
	Expect(cfg.Validate()).To(Succeed())
}

func TestOrchestratorConfigRejectsNonPositiveInterval(t *testing.T) {
	RegisterTestingT(t)

	for _, interval := range []time.Duration{0, -time.Second} {
		cfg := &OrchestratorConfig{PollInterval: interval}
		Expect(cfg.Validate()).NotTo(Succeed())
	}
}

func TestConfigLoader_OrchestratorOverrides(t *testing.T) {
	RegisterTestingT(t)
	SetMinimalTestEnv(t)
	t.Setenv("HYPERFLEET_ORCHESTRATOR_ENABLED", "true")
	t.Setenv("HYPERFLEET_ORCHESTRATOR_POLL_INTERVAL", "2s")

	cfg, err := LoadTestConfig(t)

	Expect(err).NotTo(HaveOccurred())
	Expect(cfg.Orchestrator.Enabled).To(BeTrue())
	Expect(cfg.Orchestrator.PollInterval).To(Equal(2 * time.Second))
}
