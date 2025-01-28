package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	_ "embed"

	"github.com/kairos-io/kairos-sdk/clusterplugin"
	"github.com/kairos-io/provider-rke2/pkg/constants"
	"github.com/kairos-io/provider-rke2/pkg/types"
	yip "github.com/mudler/yip/pkg/schema"
	"gopkg.in/yaml.v3"
	kyaml "sigs.k8s.io/yaml"
)

func ClusterProvider(cluster clusterplugin.Cluster) yip.YipConfig {
	var stages []yip.Stage
	clusterRootPath := getClusterRootPath(cluster)

	rke2Config := types.RKE2Config{
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

	systemName := constants.ServerSystemName
	if cluster.Role == clusterplugin.RoleWorker {
		systemName = constants.AgentSystemName
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
			Path:        filepath.Join(constants.ConfigurationPath, "90_userdata.yaml"),
			Permissions: 0400,
			Content:     string(userOptions),
		},
		{
			Path:        filepath.Join(constants.ConfigurationPath, "99_userdata.yaml"),
			Permissions: 0400,
			Content:     string(options),
		},
	}

	if len(proxyValues) > 0 {
		files = append(files, yip.File{
			Path:        filepath.Join(constants.ContainerdEnvConfigPath, systemName),
			Permissions: 0400,
			Content:     proxyValues,
		})
	}

	stages = append(stages, yip.Stage{
		Name:  "Install RKE2 Configuration Files",
		Files: files,

		Commands: []string{
			fmt.Sprintf("jq -s 'def flatten: reduce .[] as $i([]; if $i | type == \"array\" then . + ($i | flatten) else . + [$i] end); [.[] | to_entries] | flatten | reduce .[] as $dot ({}; .[$dot.key] += $dot.value)' %s/*.yaml > /etc/rancher/rke2/config.yaml", constants.ConfigurationPath),
		},
	},
	)

	if cluster.ImportLocalImages {
		if cluster.LocalImagesPath == "" {
			cluster.LocalImagesPath = constants.LocalImagesPath
		}

		importStage := yip.Stage{
			Commands: []string{
				fmt.Sprintf("/bin/sh %s/opt/rke2/scripts/import.sh %s > /var/log/import.log", clusterRootPath, cluster.LocalImagesPath),
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
		noProxy = noProxy + "," + getNodeCIDR() + "," + constants.K8SNoProxy
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

func getClusterRootPath(cluster clusterplugin.Cluster) string {
	return cluster.ProviderOptions[constants.ClusterRootPath]
}
