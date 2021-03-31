module github.com/loft-sh/loftctl

go 1.13

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/docker/docker v0.7.3-0.20190327010347-be7ac8be2ae0
	github.com/docker/cli v0.0.0-20200130152716-5d0cf8839492
	github.com/k0kubun/go-ansi v0.0.0-20180517002512-3bf9e2903213
	github.com/loft-sh/kiosk v0.1.25
	github.com/loft-sh/api v1.9.3
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pkg/errors v0.8.1
	github.com/rhysd/go-github-selfupdate v1.2.2
	github.com/sirupsen/logrus v1.4.2
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	gopkg.in/AlecAivazis/survey.v1 v1.8.8
	gopkg.in/square/go-jose.v2 v2.2.2
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/klog v1.0.0
	k8s.io/kubectl v0.20.2
)



replace github.com/kubernetes-incubator/reference-docs => github.com/kubernetes-sigs/reference-docs v0.0.0-20170929004150-fcf65347b256

replace github.com/markbates/inflect => github.com/markbates/inflect v1.0.4
