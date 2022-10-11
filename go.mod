module github.com/loft-sh/loftctl/v2

go 1.13

require (
	github.com/Microsoft/go-winio v0.5.0 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/continuity v0.1.0 // indirect
	github.com/docker/cli v0.0.0-20200130152716-5d0cf8839492
	github.com/docker/docker v0.7.3-0.20190327010347-be7ac8be2ae0
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/k0kubun/go-ansi v0.0.0-20180517002512-3bf9e2903213
	github.com/loft-sh/agentapi/v2 v2.3.1-beta.0
	github.com/loft-sh/api/v2 v2.3.1-beta.0
	github.com/loft-sh/apiserver v0.0.0-20220507140345-294e3e3117e3
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/mitchellh/go-homedir v1.1.0
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/pkg/errors v0.9.1
	github.com/rhysd/go-github-selfupdate v1.2.2
	github.com/sirupsen/logrus v1.8.1
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/cobra v1.4.0
	github.com/spf13/pflag v1.0.5
	go.uber.org/atomic v1.7.0
	gopkg.in/AlecAivazis/survey.v1 v1.8.8
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.24.0
	k8s.io/apimachinery v0.24.0
	k8s.io/client-go v0.24.0
	k8s.io/klog/v2 v2.60.1
	k8s.io/kube-aggregator v0.24.0
	k8s.io/kubectl v0.24.0
	sigs.k8s.io/controller-runtime v0.11.2
)

replace (
	github.com/loft-sh/agentapi/v2 => ../../agentapi/v2
	github.com/loft-sh/api/v2 => ../../api/v2
)
