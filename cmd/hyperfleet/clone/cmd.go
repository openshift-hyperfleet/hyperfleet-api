package clone

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type provisionCfgFlags struct {
	Name        string
	RepoBase    string
	Destination string
}

func (c *provisionCfgFlags) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Name, "name", c.Name, "Name of the new service being provisioned")
	fs.StringVar(&c.Destination, "destination", c.Destination, "Target directory for the newly provisioned instance")
	fs.StringVar(&c.RepoBase, "repo-base", c.RepoBase, "Repository base URL (e.g., 'github.com/openshift-online')")
}

var provisionCfg = &provisionCfgFlags{
	Name:        "hyperfleet",
	RepoBase:    "github.com/openshift-online",
	Destination: "/tmp/clone-test",
}

// NewCloneCommand sub-command handles cloning a new hyperfleet instance
func NewCloneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clone",
		Short: "Clone a new hyperfleet instance",
		Long:  "Clone a new hyperfleet instance",
		Run:   clone,
	}

	provisionCfg.AddFlags(cmd.PersistentFlags())
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	return cmd
}

var rw os.FileMode = 0777

func clone(_ *cobra.Command, _ []string) {

	glog.Infof("creating new hyperfleet instance as %s in directory %s", provisionCfg.Name, provisionCfg.Destination)

	originalDestination := provisionCfg.Destination

	// walk the filesystem, starting at the root of the project
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// ignore git subdirectories
		if path == ".git" || strings.Contains(path, ".git/") {
			return nil
		}

		// Replace "hyperfleet" only in the relative path
		modifiedPath := strings.ReplaceAll(path, "hyperfleet", strings.ToLower(provisionCfg.Name))
		dest := filepath.Join(originalDestination, modifiedPath)

		if info.IsDir() {
			// does this path exist in the destination?
			if _, err := os.Stat(dest); os.IsNotExist(err) {
				glog.Infof("Directory does not exist, creating: %s", dest)
			}

			err := os.MkdirAll(dest, rw)
			if err != nil {
				return err
			}

		} else {
			content, err := config.ReadFile(path)
			if err != nil {
				return err
			}

			content = strings.ReplaceAll(content, "github.com/openshift-hyperfleet/hyperfleet", "github.com/openshift-hyperfleet/hyperfleet")
			// Only replace github.com/openshift-online when it's used for generator hardcode, not for dependencies
			content = strings.ReplaceAll(content, `= "github.com/openshift-hyperfleet"`, `= "github.com/openshift-hyperfleet"`)
			content = strings.ReplaceAll(content, "ApiHyperfleetV1", "ApiHyperfleetV1")
			content = strings.ReplaceAll(content, "hyperfleet", "hyperfleet")
			content = strings.ReplaceAll(content, "hyperfleet", "hyperfleet")
			content = strings.ReplaceAll(content, "hyperfleet", "hyperfleet")
			content = strings.ReplaceAll(content, "hyperfleet", "hyperfleet")
			content = strings.ReplaceAll(content, "hyperfleet", "hyperfleet")

			replacement := fmt.Sprintf("%s/%s", provisionCfg.RepoBase, strings.ToLower(provisionCfg.Name))
			content = strings.ReplaceAll(content, "github.com/openshift-hyperfleet/hyperfleet", replacement)
			content = strings.ReplaceAll(content, `= "github.com/openshift-hyperfleet"`, fmt.Sprintf(`= "%s"`, provisionCfg.RepoBase))
			// For example, convert the service name "rh-birds" to "RhBirds"
			serviceName := strings.ToLower(provisionCfg.Name)
			parts := strings.Split(serviceName, "-")
			var titleCase string
			for _, part := range parts {
				if len(part) > 0 {
					titleCase += strings.ToUpper(string(part[0])) + part[1:]
				}
			}
			apiPrefix := fmt.Sprintf("Api%sV1", titleCase)
			content = strings.ReplaceAll(content, "ApiHyperfleetV1", apiPrefix)
			content = strings.ReplaceAll(content, "hyperfleet", provisionCfg.Name)
			content = strings.ReplaceAll(content, "hyperfleet", provisionCfg.Name)
			content = strings.ReplaceAll(content, "hyperfleet", strings.ToLower(provisionCfg.Name))
			content = strings.ReplaceAll(content, "hyperfleet", strings.ToLower(provisionCfg.Name))
			content = strings.ReplaceAll(content, "hyperfleet", strings.ToLower(provisionCfg.Name))

			if exists(dest) {
				e := os.Remove(dest)
				if e != nil {
					return e
				}
			}

			file, err := os.OpenFile(dest, os.O_APPEND|os.O_CREATE|os.O_RDWR, rw)
			if err != nil {
				return err
			}

			written, fErr := file.WriteString(content)
			if fErr != nil {
				return fErr
			}

			glog.Infof("wrote %d bytes for file %s", written, dest)
			file.Sync()
			file.Close()
		}

		return nil
	})

	if err != nil {
		fmt.Println(err)
		return
	}

	// Print next steps for the customer
	serviceName := strings.ToLower(provisionCfg.Name)
	msg := fmt.Sprintf(`
✅ Clone completed successfully!

📋 Next steps to run your new service:

1. Navigate to your new service directory:
	cd %s

2. Install dependencies:
	go mod tidy

3. Build the project:
	go install gotest.tools/gotestsum@latest
	make binary

4. Set up the database:
	make db/setup

5. Run database migrations:
	./%s migrate

6. Test the application:
	make test
	make test-integration

7. Run your service (choose one option):

	Option A: Without authentication (recommended for local development):
	make run-no-auth

	Option B: With authentication (production-like):
	make run

8. Verify the service is running:

	If using Option A (no auth):
	curl http://localhost:8000/api/%s/v1/clusters | jq

	If using Option B (with auth):
	ocm login --token=${OCM_ACCESS_TOKEN} --url=http://localhost:8000
	ocm get /api/%s/v1/clusters

For more detailed information, refer to the README.md in your new service directory.
`, provisionCfg.Destination, serviceName, serviceName, serviceName)

	fmt.Println(msg)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
