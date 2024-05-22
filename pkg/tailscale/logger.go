package tailscale

import (
	"context"
	"fmt"
	"os"
	"strings"

	"k8s.io/klog/v2"
	"tailscale.com/types/logger"
)

const (
	// EnvLogPrefix is used for enabling debug log of a specific tsnet server
	EnvLogPrefix = "LOFT_LOG_TSNET_"
)

// TsnetLogger returns a logger that logs to klog if the LOFT_LOG_TSNET_AGENT
// environment variable is set to true.
func TsnetLogger(ctx context.Context, serverName string) logger.Logf {
	logf := logger.Discard
	if os.Getenv(EnvLogPrefix+strings.ToUpper(serverName)) == "true" {
		logf = func(format string, args ...any) {
			klog.FromContext(ctx).V(1).Info("tsnet", "serverName", serverName, "msg", fmt.Sprintf(format, args...))
		}
	}
	return logf
}
