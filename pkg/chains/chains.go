package chains

import "fmt"

// Chain maps a topology/platform pair to an ordered adapter list.
type Chain struct {
	Topology string   `mapstructure:"topology" json:"topology"`
	Platform string   `mapstructure:"platform" json:"platform"`
	Adapters []string `mapstructure:"adapters" json:"adapters"`
}

// Validate checks chain structure, rejects duplicate pairs, and rejects adapter
// names that are not in knownAdapters.
func Validate(chains []Chain, knownAdapters map[string]bool) error {
	seen := make(map[string]bool, len(chains))
	for i, c := range chains {
		if c.Topology == "" {
			return fmt.Errorf("chains[%d]: topology is required", i)
		}
		if c.Platform == "" {
			return fmt.Errorf("chains[%d]: platform is required", i)
		}

		pair := c.Topology + "/" + c.Platform
		if seen[pair] {
			return fmt.Errorf("duplicate chain for %s", pair)
		}
		seen[pair] = true

		if len(c.Adapters) == 0 {
			return fmt.Errorf("chain %s requires at least one adapter", pair)
		}
		for _, name := range c.Adapters {
			if !knownAdapters[name] {
				return fmt.Errorf("chain %s references unknown adapter %q", pair, name)
			}
		}
	}
	return nil
}

// Lookup returns the adapter list for a topology/platform pair. The second
// result is false when no chain is declared for the pair.
func Lookup(chains []Chain, topology, platform string) ([]string, bool) {
	for _, c := range chains {
		if c.Topology == topology && c.Platform == platform {
			return c.Adapters, true
		}
	}
	return nil, false
}

// LookupSpec reads topology and platform from a resource spec blob.
func LookupSpec(chains []Chain, spec map[string]interface{}) ([]string, bool) {
	topology, _ := spec["topology"].(string)
	platform, _ := spec["platform"].(string)
	return Lookup(chains, topology, platform)
}
