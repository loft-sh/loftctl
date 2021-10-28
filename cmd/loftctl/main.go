package main

import (
	"os"

	"github.com/loft-sh/loftctl/cmd/loftctl/cmd"
	"github.com/loft-sh/loftctl/pkg/upgrade"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var version string = "v99.0.0"

func main() {
	upgrade.SetVersion(version)

	cmd.Execute()
	os.Exit(0)
}
