package adapters

import (
	"fmt"
	"sort"
)

// Place assigns member slots to distinct management clusters. Member 0 lands on
// preferred when set, and the remaining slots take the next clusters in name
// order. Fewer clusters than slots is an error so no partial membership is
// minted.
func Place(mcNames []string, slots int, preferred string) ([]string, error) {
	if slots <= 0 {
		slots = defaultEtcdSlots
	}
	if len(mcNames) < slots {
		return nil, fmt.Errorf(
			"insufficient management clusters for etcd placement: need %d, found %d",
			slots, len(mcNames),
		)
	}

	names := append([]string(nil), mcNames...)
	sort.Strings(names)

	first := names[0]
	if preferred != "" {
		if !contains(names, preferred) {
			return nil, fmt.Errorf("preferred management cluster %q is not in the fleet", preferred)
		}
		first = preferred
	}

	placed := make([]string, 0, slots)
	placed = append(placed, first)
	for _, name := range names {
		if name == first {
			continue
		}
		placed = append(placed, name)
		if len(placed) == slots {
			break
		}
	}
	return placed, nil
}

func contains(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
