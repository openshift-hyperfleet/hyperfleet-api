package adapters

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const (
	CoreshiftAdapterName    = "coreshift"
	defaultCoreshiftAPIPort = 6443
)

// Runner runs one coreshift CLI command and returns its combined output.
type Runner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

// ExecRunner runs the configured coreshift binary, or one found on PATH.
type ExecRunner struct {
	Binary string
}

func (r ExecRunner) Run(ctx context.Context, args ...string) (string, error) {
	binary := r.Binary
	if binary == "" {
		binary = "coreshift"
	}
	output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

// CoreshiftOptions configures the coreshift adapter.
type CoreshiftOptions struct {
	Binary         string
	PullSecretFile string
	StateDir       string
}

type createOp struct {
	done chan struct{}
	err  error
}

// CoreshiftController mints and inspects coreshift control planes.
type CoreshiftController struct {
	runner   Runner
	opts     CoreshiftOptions
	portFree func(int) bool

	mu       sync.Mutex
	creates  map[string]*createOp
	reserved map[string]int
}

var _ Controller = (*CoreshiftController)(nil)

// NewCoreshift returns a controller that talks to the coreshift CLI.
func NewCoreshift(opts CoreshiftOptions) *CoreshiftController {
	return &CoreshiftController{
		runner:   ExecRunner{Binary: opts.Binary},
		opts:     opts,
		portFree: portAvailable,
		creates:  make(map[string]*createOp),
		reserved: make(map[string]int),
	}
}

func (c *CoreshiftController) Name() string { return CoreshiftAdapterName }

func (c *CoreshiftController) Reconcile(
	ctx context.Context, cluster ClusterView,
) (StatusReport, error) {
	if cluster.Deleting {
		return pendingReport(cluster, "Deleting", "Control plane cleanup is pending"), nil
	}

	failed, running := c.takeCreate(cluster.Name)
	if running {
		return pendingReport(cluster, "Creating", "Control plane is being created"), nil
	}
	if failed != nil {
		return c.errorReport(cluster, failed), failed
	}

	clusters, err := c.listClusters(ctx)
	if err != nil {
		return c.errorReport(cluster, err), err
	}

	row, found := findCluster(clusters, cluster.Name)
	if !found {
		if err := c.startCreate(ctx, cluster.Name, clusters); err != nil {
			return c.errorReport(cluster, err), err
		}
		return pendingReport(cluster, "Creating", "Control plane creation started"), nil
	}
	if !row.ready {
		return pendingReport(cluster, "Creating", "Control plane is not ready"), nil
	}

	kubeconfig, err := c.kubeconfig(ctx, cluster.Name)
	if err != nil {
		return c.errorReport(cluster, err), err
	}
	return readyReport(cluster, kubeconfig), nil
}

// takeCreate reports an in-flight create, or returns the error of one that
// finished since the previous pass. A successful create is forgotten; its
// reserved port is kept so a recreate reuses the same one.
func (c *CoreshiftController) takeCreate(name string) (error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	op, ok := c.creates[name]
	if !ok {
		return nil, false
	}
	select {
	case <-op.done:
		delete(c.creates, name)
		if op.err != nil {
			delete(c.reserved, name)
		}
		return op.err, false
	default:
		return nil, true
	}
}

func (c *CoreshiftController) startCreate(ctx context.Context, name string, existing []clusterRow) error {
	if c.opts.PullSecretFile == "" {
		return fmt.Errorf("coreshift pull secret file is not configured")
	}

	port := c.reservePort(name, existing)
	op := &createOp{done: make(chan struct{})}

	c.mu.Lock()
	if _, exists := c.creates[name]; exists {
		c.mu.Unlock()
		return nil
	}
	c.creates[name] = op
	c.mu.Unlock()

	go func() {
		_, err := c.runner.Run(ctx, "create", "cluster",
			"--pull-secret", c.opts.PullSecretFile,
			"--name", name,
			"--port", strconv.Itoa(port),
		)
		if err != nil {
			op.err = fmt.Errorf("create coreshift cluster: %s",
				redactSecret(err.Error(), c.opts.PullSecretFile))
		}
		close(op.done)
	}()
	return nil
}

// reservePort picks the lowest free API port, leaving the following port for
// OAuth, and remembers it for this cluster name.
func (c *CoreshiftController) reservePort(name string, existing []clusterRow) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	if port, ok := c.reserved[name]; ok {
		return port
	}

	used := make(map[int]bool)
	for _, row := range existing {
		used[row.port] = true
		used[row.port+1] = true
	}
	for _, port := range c.reserved {
		used[port] = true
		used[port+1] = true
	}

	for port := defaultCoreshiftAPIPort; ; port += 2 {
		if used[port] || used[port+1] || !c.portFree(port) || !c.portFree(port+1) {
			continue
		}
		c.reserved[name] = port
		return port
	}
}

func (c *CoreshiftController) listClusters(ctx context.Context) ([]clusterRow, error) {
	output, err := c.runner.Run(ctx, "get", "clusters")
	if err != nil {
		return nil, fmt.Errorf("list coreshift clusters: %s",
			redactSecret(err.Error(), c.opts.PullSecretFile))
	}
	return parseClusters(output), nil
}

func (c *CoreshiftController) kubeconfig(ctx context.Context, name string) (string, error) {
	output, err := c.runner.Run(ctx, "get", "kubeconfig", "--name", name)
	if err != nil {
		return "", fmt.Errorf("get coreshift kubeconfig: %s",
			redactSecret(err.Error(), c.opts.PullSecretFile))
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return "", fmt.Errorf("coreshift returned an empty kubeconfig for cluster %q", name)
	}
	if !isKubeconfigContent(output) {
		return output, nil
	}
	if c.opts.StateDir == "" {
		return "", fmt.Errorf("coreshift returned kubeconfig content but no state directory is configured")
	}
	if err := os.MkdirAll(c.opts.StateDir, 0o700); err != nil {
		return "", fmt.Errorf("create coreshift state directory: %w", err)
	}

	path := filepath.Join(c.opts.StateDir, name+".kubeconfig")
	if err := os.WriteFile(path, []byte(output+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write coreshift kubeconfig: %w", err)
	}
	return path, nil
}

type clusterRow struct {
	name  string
	port  int
	ready bool
}

// parseClusters reads the tabular output of `coreshift get clusters`:
// NAME PORT RELEASE READY.
func parseClusters(output string) []clusterRow {
	var rows []clusterRow
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] == "NAME" {
			continue
		}
		port, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		rows = append(rows, clusterRow{
			name:  fields[0],
			port:  port,
			ready: fields[3] == "ready",
		})
	}
	return rows
}

func findCluster(rows []clusterRow, name string) (clusterRow, bool) {
	for _, row := range rows {
		if row.name == name {
			return row, true
		}
	}
	return clusterRow{}, false
}

func isKubeconfigContent(output string) bool {
	return strings.Contains(output, "\n") || strings.Contains(output, "apiVersion:")
}

func portAvailable(port int) bool {
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func redactSecret(text, secret string) string {
	if secret == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, "<redacted>")
}

func pendingReport(cluster ClusterView, reason, message string) StatusReport {
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionTrue, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionFalse, reason, message),
		},
	}
}

func readyReport(cluster ClusterView, kubeconfig string) StatusReport {
	const reason = "Ready"
	const message = "Control plane is ready"
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionTrue, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionTrue, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionTrue, reason, message),
		},
		Data: map[string]interface{}{"kubeconfig": kubeconfig},
	}
}

func (c *CoreshiftController) errorReport(cluster ClusterView, err error) StatusReport {
	message := redactSecret(err.Error(), c.opts.PullSecretFile)
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionFalse, "Error", message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionFalse, "Error", message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionFalse, "Error", message),
		},
	}
}

func condition(
	conditionType string, status api.AdapterConditionStatus, reason, message string,
) api.AdapterCondition {
	return api.AdapterCondition{
		Type:    conditionType,
		Status:  status,
		Reason:  &reason,
		Message: &message,
	}
}
