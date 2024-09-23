package constants

const (
	ConfigurationPath       = "/etc/rancher/rke2/config.d"
	ContainerdEnvConfigPath = "/etc/default"

	ServerSystemName = "rke2-server"
	AgentSystemName  = "rke2-agent"
	K8SNoProxy       = ".svc,.svc.cluster,.svc.cluster.local"
	LocalImagesPath  = "/opt/content/images"
)

const (
	ClusterRootPath     = "cluster_root_path"
	RunSystemdSystemDir = "/run/systemd/system"
)
