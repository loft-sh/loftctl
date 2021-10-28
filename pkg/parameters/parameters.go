package parameters

import (
	"fmt"
	"github.com/ghodss/yaml"
	managementv1 "github.com/loft-sh/api/pkg/apis/management/v1"
	storagev1 "github.com/loft-sh/api/pkg/apis/storage/v1"
	"github.com/loft-sh/loftctl/pkg/clihelper"
	"github.com/loft-sh/loftctl/pkg/log"
	"github.com/loft-sh/loftctl/pkg/survey"
	"github.com/pkg/errors"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
)

type AppFile struct {
	Apps []AppParameters `json:"apps,omitempty"`
}

type AppParameters struct {
	Name       string                 `json:"name,omitempty"`
	Parameters map[string]interface{} `json:"parameters"`
}

type NamespacedApp struct {
	App       *managementv1.App
	Namespace string
}

type NamespacedAppWithParameters struct {
	App        *managementv1.App
	Namespace  string
	Parameters string
}

func SetDeepValue(parameters interface{}, path string, value interface{}) {
	if parameters == nil {
		return
	}

	pathSegments := strings.Split(path, ".")
	switch t := parameters.(type) {
	case map[string]interface{}:
		if len(pathSegments) == 1 {
			t[pathSegments[0]] = value
			return
		}

		_, ok := t[pathSegments[0]]
		if !ok {
			t[pathSegments[0]] = map[string]interface{}{}
		}

		SetDeepValue(t[pathSegments[0]], strings.Join(pathSegments[1:], "."), value)
	}

	return
}

func GetDeepValue(parameters interface{}, path string) interface{} {
	if parameters == nil {
		return nil
	}

	pathSegments := strings.Split(path, ".")
	switch t := parameters.(type) {
	case map[string]interface{}:
		val, ok := t[pathSegments[0]]
		if !ok {
			return nil
		} else if len(pathSegments) == 1 {
			return val
		}

		return GetDeepValue(val, strings.Join(pathSegments[1:], "."))
	case []interface{}:
		index, err := strconv.Atoi(pathSegments[0])
		if err != nil {
			return nil
		} else if index < 0 || index >= len(t) {
			return nil
		}

		val := t[index]
		if len(pathSegments) == 1 {
			return val
		}

		return GetDeepValue(val, strings.Join(pathSegments[1:], "."))
	}

	return nil
}

func ResolveAppParameters(apps []NamespacedApp, appFilename string, log log.Logger) ([]NamespacedAppWithParameters, error) {
	var appFile *AppFile
	if appFilename != "" {
		out, err := ioutil.ReadFile(appFilename)
		if err != nil {
			return nil, errors.Wrap(err, "read parameters file")
		}

		appFile = &AppFile{}
		err = yaml.Unmarshal(out, appFile)
		if err != nil {
			return nil, errors.Wrap(err, "parse parameters file")
		}
	}

	ret := []NamespacedAppWithParameters{}
	for _, app := range apps {
		if len(app.App.Spec.Parameters) == 0 {
			ret = append(ret, NamespacedAppWithParameters{
				App:       app.App,
				Namespace: app.Namespace,
			})
			continue
		}

		if appFile != nil {
			parameters, err := getParametersInAppFile(app.App, appFile)
			if err != nil {
				return nil, err
			}

			ret = append(ret, NamespacedAppWithParameters{
				App:        app.App,
				Namespace:  app.Namespace,
				Parameters: parameters,
			})
			continue
		}

		log.WriteString("\n\n")
		if app.Namespace != "" {
			log.Infof("Please specify parameters for app %s in namespace %s", clihelper.GetDisplayName(app.App.Name, app.App.Spec.DisplayName), app.Namespace)
		} else {
			log.Infof("Please specify parameters for app %s", clihelper.GetDisplayName(app.App.Name, app.App.Spec.DisplayName))
		}

		parameters := map[string]interface{}{}
		for _, parameter := range app.App.Spec.Parameters {
			question := fmt.Sprintf("Parameter %s", parameter.Label)
			if parameter.Required {
				question += " (Required)"
			}

			for {
				value, err := log.Question(&survey.QuestionOptions{
					Question:     question,
					DefaultValue: parameter.DefaultValue,
					Options:      parameter.Options,
					IsPassword:   parameter.Type == "password",
				})
				if err != nil {
					return nil, err
				}

				outVal, err := verifyValue(value, parameter)
				if err != nil {
					log.Errorf(err.Error())
					continue
				}

				SetDeepValue(parameters, parameter.Variable, outVal)
				break
			}
		}

		out, err := yaml.Marshal(parameters)
		if err != nil {
			return nil, errors.Wrapf(err, "marshal app %s parameters", clihelper.GetDisplayName(app.App.Name, app.App.Spec.DisplayName))
		}
		ret = append(ret, NamespacedAppWithParameters{
			App:        app.App,
			Namespace:  app.Namespace,
			Parameters: string(out),
		})
	}

	return ret, nil
}

func verifyValue(value string, parameter storagev1.AppParameter) (interface{}, error) {
	switch parameter.Type {
	case "password":
		fallthrough
	case "enum":
		fallthrough
	case "string":
		fallthrough
	case "multiline":
		if parameter.DefaultValue != "" && value == "" {
			value = parameter.DefaultValue
		}

		if parameter.Required && value == "" {
			return nil, fmt.Errorf("parameter %s (%s) is required", parameter.Label, parameter.Variable)
		}
		for _, option := range parameter.Options {
			if option == value {
				return value, nil
			}
		}
		if parameter.Validation != "" {
			regEx, err := regexp.Compile(parameter.Validation)
			if err != nil {
				return nil, errors.Wrap(err, "compile validation regex "+parameter.Validation)
			}

			if !regEx.MatchString(value) {
				return nil, fmt.Errorf("parameter %s (%s) needs to match regex %s", parameter.Label, parameter.Variable, parameter.Validation)
			}
		}
		if parameter.Invalidation != "" {
			regEx, err := regexp.Compile(parameter.Invalidation)
			if err != nil {
				return nil, errors.Wrap(err, "compile invalidation regex "+parameter.Invalidation)
			}

			if regEx.MatchString(value) {
				return nil, fmt.Errorf("parameter %s (%s) cannot match regex %s", parameter.Label, parameter.Variable, parameter.Invalidation)
			}
		}

		return value, nil
	case "boolean":
		if parameter.DefaultValue != "" && value == "" {
			boolValue, err := strconv.ParseBool(parameter.DefaultValue)
			if err != nil {
				return nil, errors.Wrapf(err, "parse default value for parameter %s (%s)", parameter.Label, parameter.Variable)
			}

			return boolValue, nil
		}
		if parameter.Required && value == "" {
			return nil, fmt.Errorf("parameter %s (%s) is required", parameter.Label, parameter.Variable)
		}

		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			return nil, errors.Wrapf(err, "parse value for parameter %s (%s)", parameter.Label, parameter.Variable)
		}
		return boolValue, nil
	case "number":
		if parameter.DefaultValue != "" && value == "" {
			intValue, err := strconv.Atoi(parameter.DefaultValue)
			if err != nil {
				return nil, errors.Wrapf(err, "parse default value for parameter %s (%s)", parameter.Label, parameter.Variable)
			}

			return intValue, nil
		}
		if parameter.Required && value == "" {
			return nil, fmt.Errorf("parameter %s (%s) is required", parameter.Label, parameter.Variable)
		}
		num, err := strconv.Atoi(value)
		if err != nil {
			return nil, errors.Wrapf(err, "parse value for parameter %s (%s)", parameter.Label, parameter.Variable)
		}
		if parameter.Min != nil && num < *parameter.Min {
			return nil, fmt.Errorf("parameter %s (%s) cannot be smaller than %d", parameter.Label, parameter.Variable, *parameter.Min)
		}
		if parameter.Max != nil && num > *parameter.Max {
			return nil, fmt.Errorf("parameter %s (%s) cannot be greater than %d", parameter.Label, parameter.Variable, *parameter.Max)
		}

		return num, nil
	}

	return nil, fmt.Errorf("unrecognized type for paramter %s (%s): %s", parameter.Label, parameter.Variable, parameter.Type)
}

func getParametersInAppFile(appObj *managementv1.App, appFile *AppFile) (string, error) {
	if appFile == nil {
		return "", nil
	}

	for _, app := range appFile.Apps {
		if app.Name == appObj.Name {
			if app.Parameters == nil {
				app.Parameters = map[string]interface{}{}
			}

			for _, parameter := range appObj.Spec.Parameters {
				val := GetDeepValue(app.Parameters, parameter.Variable)
				strVal := ""
				if val != nil {
					switch t := val.(type) {
					case string:
						strVal = t
					case int:
						strVal = strconv.Itoa(t)
					case bool:
						strVal = strconv.FormatBool(t)
					default:
						return "", fmt.Errorf("unrecognized type for parameter %s (%s) in app %s in app file: %v", parameter.Label, parameter.Variable, clihelper.GetDisplayName(appObj.Name, appObj.Spec.DisplayName), t)
					}
				}

				outVal, err := verifyValue(strVal, parameter)
				if err != nil {
					return "", errors.Wrapf(err, "validate app %s parameters", clihelper.GetDisplayName(appObj.Name, appObj.Spec.DisplayName))
				}

				SetDeepValue(app.Parameters, parameter.Variable, outVal)
			}

			out, err := yaml.Marshal(app.Parameters)
			if err != nil {
				return "", errors.Wrapf(err, "marshal app %s parameters", clihelper.GetDisplayName(appObj.Name, appObj.Spec.DisplayName))
			}

			return string(out), nil
		}
	}

	return "", fmt.Errorf("couldn't find app %s in provided parameters file", clihelper.GetDisplayName(appObj.Name, appObj.Spec.DisplayName))
}
