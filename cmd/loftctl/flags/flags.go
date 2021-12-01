package flags

import (
	"github.com/loft-sh/loftctl/v2/pkg/client"
	flag "github.com/spf13/pflag"
)

// GlobalFlags is the flags that contains the global flags
type GlobalFlags struct {
	Silent bool
	Debug  bool
	Config string
}

// SetGlobalFlags applies the global flags
func SetGlobalFlags(flags *flag.FlagSet) *GlobalFlags {
	globalFlags := &GlobalFlags{}

	flags.StringVar(&globalFlags.Config, "config", client.DefaultCacheConfig, "The loft config to use (will be created if it does not exist)")
	flags.BoolVar(&globalFlags.Debug, "debug", false, "Prints the stack trace if an error occurs")
	flags.BoolVar(&globalFlags.Silent, "silent", false, "Run in silent mode and prevents any devspace log output except panics & fatals")

	return globalFlags
}
