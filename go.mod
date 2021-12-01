module github.com/loft-sh/loftctl/v2

go 1.13

require (
	github.com/Microsoft/go-winio v0.5.0 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/containerd/continuity v0.1.0 // indirect
	github.com/docker/cli v0.0.0-20200130152716-5d0cf8839492
	github.com/docker/docker v0.7.3-0.20190327010347-be7ac8be2ae0
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/evanphx/json-patch v4.11.0+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/k0kubun/go-ansi v0.0.0-20180517002512-3bf9e2903213
	github.com/loft-sh/agentapi/v2 v2.0.3-beta.0
	github.com/loft-sh/api/v2 v2.0.3-beta.0
	github.com/loft-sh/apimachinery/v2 v2.0.3-beta.0
	github.com/loft-sh/apiserver v0.0.0-20210607160412-10c99558fdeb
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/mitchellh/go-homedir v1.1.0
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/pkg/errors v0.9.1
	github.com/rhysd/go-github-selfupdate v1.2.2
	github.com/sirupsen/logrus v1.7.0
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	gopkg.in/AlecAivazis/survey.v1 v1.8.8
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
	k8s.io/klog v1.0.0
	k8s.io/kube-aggregator v0.21.1
	k8s.io/kubectl v0.21.1
	sigs.k8s.io/controller-runtime v0.9.0
)

replace (
	github.com/go-openapi/jsonpointer => github.com/go-openapi/jsonpointer v0.19.3
	github.com/go-openapi/jsonreference => github.com/go-openapi/jsonreference v0.19.3
	github.com/go-openapi/swag => github.com/go-openapi/swag v0.19.5
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
	github.com/kubernetes-incubator/reference-docs => github.com/kubernetes-sigs/reference-docs v0.0.0-20170929004150-fcf65347b256
	
	
	
	github.com/markbates/inflect => github.com/markbates/inflect v1.0.4
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
)
