package main

import (
	"os"

	"github.com/loft-sh/loftctl/v2/cmd/loftctl/cmd"
	"github.com/loft-sh/loftctl/v2/pkg/upgrade"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var version string = "v99.0.0"

func main() {
	upgrade.SetVersion(version)

	cmd.Execute()
	os.Exit(0)
}
