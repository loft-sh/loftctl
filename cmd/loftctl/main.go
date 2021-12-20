package main

import (
	"github.com/loft-sh/loftctl/cmd/loftctl/cmd"
	"github.com/loft-sh/loftctl/pkg/upgrade"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var version string

func main() {
	upgrade.SetVersion(version)

	cmd.Execute()
	os.Exit(0)
}
