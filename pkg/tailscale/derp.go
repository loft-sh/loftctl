package tailscale

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/loft-sh/loftctl/v4/pkg/httputil"
	"k8s.io/klog/v2"
)

// CheckDerpConnection takes a baseUrl and probes whether a derp connection can be established.
func CheckDerpConnection(ctx context.Context, baseUrl *url.URL) error {
	newTransport := httputil.CloneDefaultTransport()
	newTransport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: os.Getenv("TS_DEBUG_TLS_DIAL_INSECURE_SKIP_VERIFY") == "true",
	}

	c := &http.Client{
		Transport: newTransport,
		Timeout:   5 * time.Second,
	}

	derpUrl := *baseUrl
	derpUrl.Path = "/derp/probe"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, derpUrl.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	res, err := c.Do(req)
	if err != nil || (res != nil && res.StatusCode != http.StatusOK) {
		klog.FromContext(ctx).Error(err, "Failed to reach the coordinator server. Make sure that the agent can reach the control-plane. Also, make sure to try using `insecureSkipVerify` or `additionalCA` in the control-plane's helm values in case of x509 certificate issues.", "derpUrl", derpUrl.String())

		if res != nil {
			body, _ := io.ReadAll(res.Body)
			defer res.Body.Close()

			klog.FromContext(ctx).V(1).Info("Details", "error", err, "statusCode", res.StatusCode, "body", string(body))
		}

		return fmt.Errorf("failed to reach the coordinator server: %w", err)
	}

	return nil
}
