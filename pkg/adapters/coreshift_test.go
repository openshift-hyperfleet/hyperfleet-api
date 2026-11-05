package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const testPullSecret = "/outside/repo/pull-secret"

type fakeCoreshiftRunner struct {
	run   func(args []string) (string, error)
	mu    sync.Mutex
	calls [][]string
}

func (r *fakeCoreshiftRunner) Run(_ context.Context, args ...string) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, append([]string(nil), args...))
	r.mu.Unlock()
	return r.run(args)
}

func (r *fakeCoreshiftRunner) snapshot() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]string(nil), r.calls...)
}

func newTestCoreshift(runner Runner, opts CoreshiftOptions) *CoreshiftController {
	controller := NewCoreshift(opts)
	controller.runner = runner
	controller.portFree = func(int) bool { return true }
	return controller
}

func testOptions() CoreshiftOptions {
	return CoreshiftOptions{
		Binary:         "coreshift",
		PullSecretFile: testPullSecret,
		StateDir:       "/outside/repo/state",
	}
}

func conditionStatus(report StatusReport, conditionType string) api.AdapterConditionStatus {
	condition := conditionByType(report, conditionType)
	Expect(condition).ToNot(BeNil(), conditionType)
	return condition.Status
}

func waitForCreate(t *testing.T, controller *CoreshiftController, name string) {
	t.Helper()
	controller.mu.Lock()
	op := controller.creates[name]
	controller.mu.Unlock()
	Expect(op).ToNot(BeNil())
	select {
	case <-op.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for coreshift create")
	}
}

func TestCoreshiftReportsReadyWithKubeconfigPath(t *testing.T) {
	RegisterTestingT(t)
	kubeconfig := "/outside/repo/state/my-cluster.kubeconfig"
	runner := &fakeCoreshiftRunner{run: func(args []string) (string, error) {
		switch strings.Join(args, " ") {
		case "get clusters":
			return "NAME PORT RELEASE READY\nmy-cluster 6443 4.22 ready\n", nil
		case "get kubeconfig --name my-cluster":
			return kubeconfig, nil
		}
		return "", fmt.Errorf("unexpected args: %v", args)
	}}
	controller := newTestCoreshift(runner, testOptions())
	cluster := ClusterView{Name: "my-cluster", Generation: 7}

	report, err := controller.Reconcile(context.Background(), cluster)

	Expect(err).ToNot(HaveOccurred())
	Expect(report.ObservedGeneration).To(Equal(cluster.Generation))
	Expect(report.Data).To(HaveKeyWithValue("kubeconfig", kubeconfig))
	for _, conditionType := range []string{
		api.AdapterConditionTypeAvailable,
		api.AdapterConditionTypeApplied,
		api.AdapterConditionTypeHealth,
	} {
		Expect(conditionStatus(report, conditionType)).To(Equal(api.AdapterConditionTrue))
	}
	Expect(runner.snapshot()).To(HaveLen(2))
}

func TestCoreshiftReportsPendingForExistingNotReadyCluster(t *testing.T) {
	RegisterTestingT(t)
	runner := &fakeCoreshiftRunner{run: func(args []string) (string, error) {
		Expect(strings.Join(args, " ")).To(Equal("get clusters"))
		return "NAME PORT RELEASE READY\nmy-cluster 6443 4.22 not ready\n", nil
	}}
	controller := newTestCoreshift(runner, testOptions())

	report, err := controller.Reconcile(context.Background(), ClusterView{Name: "my-cluster", Generation: 3})

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(report, api.AdapterConditionTypeAvailable)).To(Equal(api.AdapterConditionFalse))
	Expect(conditionStatus(report, api.AdapterConditionTypeApplied)).To(Equal(api.AdapterConditionTrue))
	Expect(conditionStatus(report, api.AdapterConditionTypeHealth)).To(Equal(api.AdapterConditionFalse))
	Expect(runner.snapshot()).To(HaveLen(1))
}

func TestCoreshiftStartsCreateOnceThenReportsReady(t *testing.T) {
	RegisterTestingT(t)
	started := make(chan []string, 1)
	release := make(chan struct{})
	var listReady atomic.Bool
	runner := &fakeCoreshiftRunner{run: func(args []string) (string, error) {
		switch args[0] {
		case "get":
			if args[1] == "kubeconfig" {
				return "/outside/repo/my-cluster.kubeconfig", nil
			}
			if listReady.Load() {
				return "NAME PORT RELEASE READY\nmy-cluster 6443 4.22 ready\n", nil
			}
			return "No clusters\n", nil
		case "create":
			started <- append([]string(nil), args...)
			<-release
			return "", nil
		}
		return "", fmt.Errorf("unexpected args: %v", args)
	}}
	controller := newTestCoreshift(runner, testOptions())
	cluster := ClusterView{Name: "my-cluster", Generation: 1}

	first, err := controller.Reconcile(context.Background(), cluster)
	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(first, api.AdapterConditionTypeAvailable)).To(Equal(api.AdapterConditionFalse))

	createArgs := <-started
	Expect(createArgs).To(Equal([]string{
		"create", "cluster",
		"--pull-secret", testPullSecret,
		"--name", "my-cluster",
		"--port", "6443",
	}))

	second, err := controller.Reconcile(context.Background(), cluster)
	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(second, api.AdapterConditionTypeAvailable)).To(Equal(api.AdapterConditionFalse))

	createCalls := 0
	for _, call := range runner.snapshot() {
		if call[0] == "create" {
			createCalls++
		}
	}
	Expect(createCalls).To(Equal(1))

	close(release)
	waitForCreate(t, controller, cluster.Name)
	listReady.Store(true)

	report, err := controller.Reconcile(context.Background(), cluster)
	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(report, api.AdapterConditionTypeAvailable)).To(Equal(api.AdapterConditionTrue))
	Expect(report.Data).To(HaveKeyWithValue("kubeconfig", "/outside/repo/my-cluster.kubeconfig"))
}

func TestCoreshiftAllocatesNextPortWhenDefaultIsUsed(t *testing.T) {
	RegisterTestingT(t)
	started := make(chan []string, 1)
	release := make(chan struct{})
	runner := &fakeCoreshiftRunner{run: func(args []string) (string, error) {
		switch args[0] {
		case "get":
			return "NAME PORT RELEASE READY\nother 6443 4.22 ready\n", nil
		case "create":
			started <- append([]string(nil), args...)
			<-release
			return "", nil
		}
		return "", fmt.Errorf("unexpected args: %v", args)
	}}
	controller := newTestCoreshift(runner, testOptions())

	_, err := controller.Reconcile(context.Background(), ClusterView{Name: "new-cluster", Generation: 1})
	Expect(err).ToNot(HaveOccurred())

	createArgs := <-started
	Expect(createArgs).To(ContainElements("--port", "6445"))
	close(release)
	waitForCreate(t, controller, "new-cluster")
}

func TestCoreshiftPersistsKubeconfigContent(t *testing.T) {
	RegisterTestingT(t)
	stateDir := t.TempDir()
	kubeconfig := "apiVersion: v1\nclusters: []\n"
	runner := &fakeCoreshiftRunner{run: func(args []string) (string, error) {
		switch strings.Join(args, " ") {
		case "get clusters":
			return "NAME PORT RELEASE READY\nmy-cluster 6443 4.22 ready\n", nil
		case "get kubeconfig --name my-cluster":
			return kubeconfig, nil
		}
		return "", fmt.Errorf("unexpected args: %v", args)
	}}
	opts := testOptions()
	opts.StateDir = stateDir
	controller := newTestCoreshift(runner, opts)

	report, err := controller.Reconcile(context.Background(), ClusterView{Name: "my-cluster", Generation: 2})
	Expect(err).ToNot(HaveOccurred())

	path := filepath.Join(stateDir, "my-cluster.kubeconfig")
	Expect(report.Data).To(HaveKeyWithValue("kubeconfig", path))
	written, readErr := os.ReadFile(path)
	Expect(readErr).ToNot(HaveOccurred())
	Expect(string(written)).To(Equal(strings.TrimSpace(kubeconfig) + "\n"))
}

func TestCoreshiftReportsCreateFailureWithoutLeakingPullSecret(t *testing.T) {
	RegisterTestingT(t)
	release := make(chan struct{})
	runner := &fakeCoreshiftRunner{run: func(args []string) (string, error) {
		switch args[0] {
		case "get":
			return "No clusters\n", nil
		case "create":
			<-release
			return "", fmt.Errorf("cannot read %s", testPullSecret)
		}
		return "", fmt.Errorf("unexpected args: %v", args)
	}}
	controller := newTestCoreshift(runner, testOptions())
	cluster := ClusterView{Name: "my-cluster", Generation: 2}

	_, err := controller.Reconcile(context.Background(), cluster)
	Expect(err).ToNot(HaveOccurred())

	close(release)
	waitForCreate(t, controller, cluster.Name)

	report, err := controller.Reconcile(context.Background(), cluster)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).ToNot(ContainSubstring(testPullSecret))
	Expect(err.Error()).To(ContainSubstring("<redacted>"))
	for _, conditionType := range []string{
		api.AdapterConditionTypeAvailable,
		api.AdapterConditionTypeApplied,
		api.AdapterConditionTypeHealth,
	} {
		Expect(conditionStatus(report, conditionType)).To(Equal(api.AdapterConditionFalse))
	}
}

func TestParseClustersReadsReadiness(t *testing.T) {
	RegisterTestingT(t)

	rows := parseClusters("NAME PORT RELEASE READY\n" +
		"ready-one 6443 4.22 ready\n" +
		"booting 6445 - not ready\n")

	Expect(rows).To(Equal([]clusterRow{
		{name: "ready-one", port: 6443, ready: true},
		{name: "booting", port: 6445, ready: false},
	}))
	Expect(parseClusters("No clusters\n")).To(BeEmpty())
}
