package main

import (
	"github.com/kairos-io/kairos-sdk/clusterplugin"
	"github.com/kairos-io/provider-rke2/pkg/log"
	"github.com/kairos-io/provider-rke2/pkg/provider"
	"github.com/mudler/go-pluggable"
	"github.com/sirupsen/logrus"
)

func main() {
	log.InitLogger("/var/log/provider-rke2.log")

	plugin := clusterplugin.ClusterPlugin{
		Provider: provider.ClusterProvider,
	}

	if err := plugin.Run(pluggable.FactoryPlugin{
		EventType:     clusterplugin.EventClusterReset,
		PluginHandler: provider.HandleClusterReset,
	}); err != nil {
		logrus.Fatal(err)
	}
}
