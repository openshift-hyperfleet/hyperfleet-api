package fleet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const kubeconfigExt = ".kubeconfig"

// MC is a management cluster discovered from the fleet directory.
type MC struct {
	Name       string
	Kubeconfig string
}

// List returns the management clusters in dir on every call. The MC set is
// exactly the set of *.kubeconfig files; nothing is cached or owned.
func List(dir string) ([]MC, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read fleet directory %q: %w", dir, err)
	}

	clusters := make([]MC, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != kubeconfigExt {
			continue
		}
		clusters = append(clusters, MC{
			Name:       strings.TrimSuffix(entry.Name(), kubeconfigExt),
			Kubeconfig: filepath.Join(dir, entry.Name()),
		})
	}
	return clusters, nil
}
