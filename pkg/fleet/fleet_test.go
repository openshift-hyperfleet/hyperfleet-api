package fleet

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestListReadsKubeconfigFiles(t *testing.T) {
	RegisterTestingT(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "etcd-mc-1.kubeconfig"))
	writeFile(t, filepath.Join(dir, "etcd-mc-0.kubeconfig"))
	writeFile(t, filepath.Join(dir, "notes.txt"))
	Expect(os.Mkdir(filepath.Join(dir, "nested.kubeconfig"), 0o755)).To(Succeed())

	clusters, err := List(dir)

	Expect(err).ToNot(HaveOccurred())
	Expect(clusters).To(Equal([]MC{
		{Name: "etcd-mc-0", Kubeconfig: filepath.Join(dir, "etcd-mc-0.kubeconfig")},
		{Name: "etcd-mc-1", Kubeconfig: filepath.Join(dir, "etcd-mc-1.kubeconfig")},
	}))
}

func TestListReflectsCurrentDirectoryContents(t *testing.T) {
	RegisterTestingT(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "etcd-mc-0.kubeconfig")
	second := filepath.Join(dir, "etcd-mc-1.kubeconfig")
	writeFile(t, first)

	clusters, err := List(dir)
	Expect(err).ToNot(HaveOccurred())
	Expect(clusters).To(HaveLen(1))

	writeFile(t, second)
	clusters, err = List(dir)
	Expect(err).ToNot(HaveOccurred())
	Expect(clusters).To(HaveLen(2))

	Expect(os.Remove(first)).To(Succeed())
	clusters, err = List(dir)
	Expect(err).ToNot(HaveOccurred())
	Expect(clusters).To(HaveLen(1))
	Expect(clusters[0].Name).To(Equal("etcd-mc-1"))
}

func TestListRejectsMissingDirectory(t *testing.T) {
	RegisterTestingT(t)
	_, err := List(filepath.Join(t.TempDir(), "missing"))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("read fleet directory"))
}

func TestListEmptyDirectory(t *testing.T) {
	RegisterTestingT(t)
	clusters, err := List(t.TempDir())
	Expect(err).ToNot(HaveOccurred())
	Expect(clusters).To(BeEmpty())
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	Expect(os.WriteFile(path, []byte("fake kubeconfig\n"), 0o600)).To(Succeed())
}
