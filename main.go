package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"github.com/kairos-io/kairos-sdk/clusterplugin"
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
	K8SNoProxy       = ".svc,.svc.cluster,.svc.cluster.local"
	localImagesPath  = "/opt/content/images"
)

type RKE2Config struct {
	Token  string   `yaml:"token"`
	Server string   `yaml:"server"`
	TLSSan []string `yaml:"tls-san"`
}

func clusterProvider(cluster clusterplugin.Cluster) yip.YipConfig {

	var importStage yip.Stage
	rke2Config := RKE2Config{
		Token: cluster.ClusterToken,
		// RKE2 server listens on 9345 for node registration https://docs.rke2.io/install/quickstart/#3-configure-the-rke2-agent-service
		Server: fmt.Sprintf("https://%s:9345", cluster.ControlPlaneHost),
		TLSSan: []string{
			cluster.ControlPlaneHost,
		},
	}

	if cluster.Role == clusterplugin.RoleInit {
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

	proxyValues := proxyEnv(userOptions, cluster.Env)

	files := []yip.File{
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
	}

	if len(proxyValues) > 0 {
		files = append(files, yip.File{
			Path:        filepath.Join(containerdEnvConfigPath, systemName),
			Permissions: 0400,
			Content:     proxyValues,
		})
	}

	stages := []yip.Stage{
		{
			Name:  "Install RKE2 Configuration Files",
			Files: files,

			Commands: []string{
				fmt.Sprintf("jq -s 'def flatten: reduce .[] as $i([]; if $i | type == \"array\" then . + ($i | flatten) else . + [$i] end); [.[] | to_entries] | flatten | reduce .[] as $dot ({}; .[$dot.key] += $dot.value)' %s/*.yaml > /etc/rancher/rke2/config.yaml", configurationPath),
			},
		},
	}

	if cluster.ImportLocalImages {
		if cluster.LocalImagesPath == "" {
			cluster.LocalImagesPath = localImagesPath
		}

		importStage = yip.Stage{
			Commands: []string{
				fmt.Sprintf("/bin/sh /opt/rke2/scripts/import.sh %s > /var/log/import.log", cluster.LocalImagesPath),
			},
			If: fmt.Sprintf("[  -d %s ]", cluster.LocalImagesPath),
		}
		stages = append(stages, importStage)
	}

	stages = append(stages,
		yip.Stage{
			Name: "Waiting to finish extracting content",
			Commands: []string{
				"sleep 120",
			},
		},
		yip.Stage{
			Name: "Enable Systemd Services",
			Commands: []string{
				fmt.Sprintf("systemctl enable %s", systemName),
				fmt.Sprintf("systemctl restart %s", systemName),
			},
		})

	cfg := yip.YipConfig{
		Name: "RKE2 Kairos Cluster Provider",
		Stages: map[string][]yip.Stage{
			"boot.before": stages,
		},
	}

	return cfg
}

func proxyEnv(userOptions []byte, proxyMap map[string]string) string {
	var proxy []string
	var noProxy string
	var isProxyConfigured bool

	httpProxy := proxyMap["HTTP_PROXY"]
	httpsProxy := proxyMap["HTTPS_PROXY"]
	userNoProxy := proxyMap["NO_PROXY"]
	defaultNoProxy := getDefaultNoProxy(userOptions)

	if len(httpProxy) > 0 {
		proxy = append(proxy, fmt.Sprintf("HTTP_PROXY=%s", httpProxy))
		proxy = append(proxy, fmt.Sprintf("CONTAINERD_HTTP_PROXY=%s", httpProxy))
		isProxyConfigured = true
	}

	if len(httpsProxy) > 0 {
		proxy = append(proxy, fmt.Sprintf("HTTPS_PROXY=%s", httpsProxy))
		proxy = append(proxy, fmt.Sprintf("CONTAINERD_HTTPS_PROXY=%s", httpsProxy))
		isProxyConfigured = true
	}

	if isProxyConfigured {
		noProxy = defaultNoProxy
	}

	if len(userNoProxy) > 0 {
		noProxy = noProxy + "," + userNoProxy
	}

	if len(noProxy) > 0 {
		proxy = append(proxy, fmt.Sprintf("NO_PROXY=%s", noProxy))
		proxy = append(proxy, fmt.Sprintf("CONTAINERD_NO_PROXY=%s", noProxy))
	}

	return strings.Join(proxy, "\n")
}

func getDefaultNoProxy(userOptions []byte) string {

	var noProxy string

	data := make(map[string]interface{})
	err := json.Unmarshal(userOptions, &data)
	if err != nil {
		fmt.Println("error while unmarshalling user options", err)
	}

	if data != nil {
		clusterCIDR := data["cluster-cidr"].(string)
		serviceCIDR := data["service-cidr"].(string)

		if len(clusterCIDR) > 0 {
			noProxy = noProxy + "," + clusterCIDR
		}
		if len(serviceCIDR) > 0 {
			noProxy = noProxy + "," + serviceCIDR
		}
		noProxy = noProxy + "," + getNodeCIDR() + "," + K8SNoProxy
	}

	return noProxy
}

func getNodeCIDR() string {
	addrs, _ := net.InterfaceAddrs()
	var result string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				result = addr.String()
				break
			}
		}
	}
	return result
}

func main() {
	plugin := clusterplugin.ClusterPlugin{
		Provider: clusterProvider,
	}

	if err := plugin.Run(); err != nil {
		logrus.Fatal(err)
	}
}
