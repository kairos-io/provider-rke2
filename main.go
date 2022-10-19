package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/c3os-io/c3os/sdk/clusterplugin"
	yip "github.com/mudler/yip/pkg/schema"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	kyaml "sigs.k8s.io/yaml"
)

const (
	configurationPath       = "/etc/rancher/rke2/config.d"
	containerdEnvConfigPath = "/etc/default"

	serverSystemName = "rke2-server"
	agentSystemName  = "rke2-agent"
)

type RKE2Config struct {
	ClusterInit bool     `yaml:"cluster-init"`
	Token       string   `yaml:"token"`
	Server      string   `yaml:"server"`
	TLSSan      []string `yaml:"tls-san"`
}

func clusterProvider(cluster clusterplugin.Cluster) yip.YipConfig {
	rke2Config := RKE2Config{
		Token: cluster.ClusterToken,
		// RKE2 server listens on 9345 for node registration https://docs.rke2.io/install/quickstart/#3-configure-the-rke2-agent-service
		Server: fmt.Sprintf("https://%s:9345", cluster.ControlPlaneHost),
		TLSSan: []string{
			cluster.ControlPlaneHost,
		},
	}

	if cluster.Role == clusterplugin.RoleInit {
		rke2Config.ClusterInit = true
		rke2Config.Server = ""

	}

	systemName := serverSystemName
	if cluster.Role == clusterplugin.RoleWorker {
		systemName = agentSystemName
	}

	// ensure we always have  a valid user config
	if cluster.Options == "" {
		cluster.Options = "{}"
	}

	var providerConfig bytes.Buffer
	_ = yaml.NewEncoder(&providerConfig).Encode(&rke2Config)

	userOptions, _ := kyaml.YAMLToJSON([]byte(cluster.Options))
	options, _ := kyaml.YAMLToJSON(providerConfig.Bytes())

	cfg := yip.YipConfig{
		Name: "RKE2 C3OS Cluster Provider",
		Stages: map[string][]yip.Stage{
			"boot.before": {
				{
					Name: "Install RKE2 Configuration Files",
					Files: []yip.File{
						{
							Path:        filepath.Join(configurationPath, "90_userdata.yaml"),
							Permissions: 0400,
							Content:     string(userOptions),
						},
						{
							Path:        filepath.Join(configurationPath, "99_userdata.yaml"),
							Permissions: 0400,
							Content:     string(options),
						},
						{
							Path:        filepath.Join(containerdEnvConfigPath, systemName),
							Permissions: 0400,
							Content:     containerdProxyEnv(),
						},
					},
					Commands: []string{
						fmt.Sprintf("jq -s 'def flatten: reduce .[] as $i([]; if $i | type == \"array\" then . + ($i | flatten) else . + [$i] end); [.[] | to_entries] | flatten | reduce .[] as $dot ({}; .[$dot.key] += $dot.value)' %s/*.yaml > /etc/rancher/rke2/config.yaml", configurationPath),
					},
				},
				{
					Name: "Enable Systemd Services",
					Systemctl: yip.Systemctl{
						Enable: []string{
							systemName,
						},
						Start: []string{
							systemName,
						},
					},
				},
			},
		},
	}

	return cfg
}

func containerdProxyEnv() string {
	var proxy []string
	if len(os.Getenv("HTTP_PROXY")) > 0 {
		proxy = append(proxy, "CONTAINERD_HTTP_PROXY=${HTTP_PROXY}")
	}

	if len(os.Getenv("HTTPS_PROXY")) > 0 {
		proxy = append(proxy, "CONTAINERD_HTTPS_PROXY=${HTTPS_PROXY}")
	}

	if len(os.Getenv("NO_PROXY")) > 0 {
		proxy = append(proxy, "CONTAINERD_NO_PROXY=${NO_PROXY}")
	}

	return strings.Join(proxy, "\n")
}

func main() {
	plugin := clusterplugin.ClusterPlugin{
		Provider: clusterProvider,
	}

	if err := plugin.Run(); err != nil {
		logrus.Fatal(err)
	}
}
