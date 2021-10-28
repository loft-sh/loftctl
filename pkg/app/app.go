package app

import (
	clusterv1 "github.com/loft-sh/agentapi/pkg/apis/loft/cluster/v1"
	managementv1 "github.com/loft-sh/api/pkg/apis/management/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
)

const (
	// LoftHelmReleaseAppLabel indicates if the helm release was deployed via the loft app store
	LoftHelmReleaseAppLabel = "loft.sh/app"

	// LoftHelmReleaseAppNameLabel indicates if the helm release was deployed via the loft app store
	LoftHelmReleaseAppNameLabel = "loft.sh/app-name"

	// LoftHelmReleaseAppGenerationAnnotation indicates the resource version of the loft app
	LoftHelmReleaseAppGenerationAnnotation = "loft.sh/app-generation"

	// LoftDefaultSpaceTemplate indicates the default space template on a cluster
	LoftDefaultSpaceTemplate = "space.loft.sh/default-template"
)

func ConvertAppToHelmRelease(app *managementv1.App, namespace string) *clusterv1.HelmRelease {
	release := &clusterv1.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: namespace,
			Labels: map[string]string{
				LoftHelmReleaseAppLabel:     "true",
				LoftHelmReleaseAppNameLabel: app.Name,
			},
			Annotations: map[string]string{
				LoftHelmReleaseAppGenerationAnnotation: strconv.FormatInt(app.Generation, 10),
			},
		},
		Spec: clusterv1.HelmReleaseSpec{
			Manifests: app.Spec.Manifests,
		},
	}
	if app.Spec.ReleaseName != "" {
		release.Name = app.Spec.ReleaseName
	}
	if app.Spec.Helm != nil {
		release.Spec.Chart = clusterv1.Chart{
			Name:     app.Spec.Helm.Name,
			Version:  app.Spec.Helm.Version,
			RepoURL:  app.Spec.Helm.RepoURL,
			Username: app.Spec.Helm.Username,
			Password: string(app.Spec.Helm.Password),
		}
		release.Spec.Config = app.Spec.Helm.Values
		release.Spec.InsecureSkipTlsVerify = app.Spec.Helm.Insecure
	}
	return release
}
