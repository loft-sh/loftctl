package client

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Config defines the client config structure
type Config struct {
	metav1.TypeMeta `json:",inline"`

	// host is the http endpoint of how to access loft
	// +optional
	Host string `json:"host,omitempty"`

	// LastInstallContext is the last install context
	// +optional
	LastInstallContext string `json:"lastInstallContext,omitempty"`

	// insecure specifies if the loft instance is insecure
	// +optional
	Insecure bool `json:"insecure,omitempty"`

	// accesskey is the accesskey for the given loft host
	// +optional
	AccessKey string `json:"accesskey,omitempty"`
}

// NewConfig creates a new config
func NewConfig() *Config {
	return &Config{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Config",
			APIVersion: "storage.loft.sh/v1",
		},
	}
}
