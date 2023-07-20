package main

import (
	"os"

	"github.com/loft-sh/loftctl/v3/cmd/loftctl/cmd"
	"github.com/loft-sh/loftctl/v3/pkg/upgrade"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
)

var version string = "v99.0.0"

func main() {
	err := upgrade.SetVersion(version)
	if err != nil {
		klog.TODO().Error(err, "error setting version")
	}

	cmd.Execute()
	os.Exit(0)
}
