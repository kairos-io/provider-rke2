package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kairos-io/kairos-sdk/clusterplugin"

	"github.com/kairos-io/provider-rke2/pkg/constants"
)

func Test_systemdStageDoesNotPersistEnablement(t *testing.T) {
	for _, role := range []clusterplugin.Role{clusterplugin.RoleInit, clusterplugin.RoleWorker} {
		t.Run(string(role), func(t *testing.T) {
			cluster := clusterplugin.Cluster{
				ClusterToken:     "token",
				ControlPlaneHost: "localhost",
				Role:             role,
				Options:          "cluster-cidr: 10.42.0.0/16\nservice-cidr: 10.43.0.0/16\n",
			}
			systemName := constants.ServerSystemName
			if role == clusterplugin.RoleWorker {
				systemName = constants.AgentSystemName
			}

			cfg := ClusterProvider(cluster)

			var commands []string
			for _, stage := range cfg.Stages["boot.before"] {
				if stage.Name == "Enable Systemd Services" {
					commands = stage.Commands
				}
			}
			if commands == nil {
				t.Fatalf("no %q stage found", "Enable Systemd Services")
			}

			joined := strings.Join(commands, "\n")
			if strings.Contains(joined, fmt.Sprintf("systemctl enable %s", systemName)) {
				t.Errorf("stage persistently enables %s; systemd would auto-start it before config.yaml is rendered:\n%s", systemName, joined)
			}
			if !strings.Contains(joined, fmt.Sprintf("systemctl enable --runtime %s", systemName)) {
				t.Errorf("stage does not runtime-enable %s:\n%s", systemName, joined)
			}
			if !strings.Contains(joined, fmt.Sprintf("systemctl disable %s", systemName)) {
				t.Errorf("stage does not clear a pre-existing persistent link for %s:\n%s", systemName, joined)
			}
		})
	}
}
