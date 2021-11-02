package app

import (
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
			Config: app.Spec.Config,
		},
		StreamContainer: app.Spec.StreamContainer,
	}
	if helmTask.Release.Config.Annotations == nil {
		helmTask.Release.Config.Annotations = map[string]string{}
	}
	helmTask.Release.Config.Annotations[LoftHelmReleaseAppNameAnnotation] = app.Name
	helmTask.Release.Config.Annotations[LoftHelmReleaseAppGenerationAnnotation] = strconv.FormatInt(app.Generation, 10)
	if app.Spec.Wait {
		helmTask.Args = append(helmTask.Args, "--wait")
	}
	if app.Spec.Timeout != "" {
		helmTask.Args = append(helmTask.Args, "--timeout", app.Spec.Timeout)
	}
	return helmTask
}
