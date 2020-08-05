package client

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Config defines the client config structure
type Config struct {
	metav1.TypeMeta `json:",inline"`

	// host is the http endpoint of how to access loft
	// +optional
	Host string `json:"host,omitempty"`

	// insecure specifies if the loft instance is insecure
	// +optional
	Insecure bool `json:"insecure,omitempty"`

	// username is the username of the logged in user
	// +optional
	Username string `json:"username"`

	// accesskey is the accesskey for the given loft host
	// +optional
	AccessKey string `json:"accesskey,omitempty"`

	// oidc token is the oidc token to retrieve a loft token from
	// +optional
	OIDCToken string `json:"oidcToken,omitempty"`

	// oidc refresh token is the oidc refresh token to retrieve a new
	// oidc token from
	// +optional
	OIDCRefreshToken string `json:"oidcRefreshToken,omitempty"`

	// token is the login token for the given loft host
	// +optional
	Token string `json:"token,omitempty"`

	// tokenExp is the local time the token will expire
	// +optional
	TokenExp int64 `json:"tokenExp,omitempty"`
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
