package app

import (
	clusterv1 "github.com/loft-sh/agentapi/pkg/apis/loft/cluster/v1"
	managementv1 "github.com/loft-sh/api/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/pkg/apis/storage/v1"
	"strconv"
)

const (
	// LoftHelmReleaseAppLabel indicates if the helm release was deployed via the loft app store
	LoftHelmReleaseAppLabel = "loft.sh/app"

	// LoftHelmReleaseAppNameAnnotation indicates if the helm release was deployed via the loft app store
	LoftHelmReleaseAppNameAnnotation = "loft.sh/app-name"

	// LoftHelmReleaseAppGenerationAnnotation indicates the resource version of the loft app
	LoftHelmReleaseAppGenerationAnnotation = "loft.sh/app-generation"

	// LoftDefaultSpaceTemplate indicates the default space template on a cluster
	LoftDefaultSpaceTemplate = "space.loft.sh/default-template"
)

func ConvertAppToHelmTask(app *managementv1.App, namespace string) *storagev1.HelmTask {
	helmTask := &storagev1.HelmTask{
		Type: storagev1.HelmTaskTypeInstall,
		Release: storagev1.HelmTaskRelease{
			Name:      app.Name,
			Namespace: namespace,
			Labels: map[string]string{
				LoftHelmReleaseAppLabel: "true",
			},
			Annotations: map[string]string{
				LoftHelmReleaseAppNameAnnotation:       app.Name,
				LoftHelmReleaseAppGenerationAnnotation: strconv.FormatInt(app.Generation, 10),
			},
		},
		Helm: storagev1.HelmTaskTemplate{
			Manifests: app.Spec.Manifests,
		},
	}
	if app.Spec.ReleaseName != "" {
		helmTask.Release.Name = app.Spec.ReleaseName
	}
	if app.Spec.Helm != nil {
		helmTask.Helm.Chart = clusterv1.Chart{
			Name:     app.Spec.Helm.Name,
			Version:  app.Spec.Helm.Version,
			RepoURL:  app.Spec.Helm.RepoURL,
			Username: app.Spec.Helm.Username,
			Password: string(app.Spec.Helm.Password),
		}
		helmTask.Helm.Config = app.Spec.Helm.Values
		helmTask.Helm.InsecureSkipTlsVerify = app.Spec.Helm.Insecure
	}
	if app.Spec.Wait {
		helmTask.Args = append(helmTask.Args, "--wait")
	}
	if app.Spec.Timeout != "" {
		helmTask.Args = append(helmTask.Args, "--timeout", app.Spec.Timeout)
	}
	return helmTask
}
