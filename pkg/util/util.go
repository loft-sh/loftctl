package util

import (
	"k8s.io/apimachinery/pkg/api/errors"
)

func GetCause(err error) string {
	if err == nil {
		return ""
	}

	if statusErr, ok := err.(*errors.StatusError); ok {
		details := statusErr.Status().Details
		if details != nil && len(details.Causes) > 0 {
			return details.Causes[0].Message
		}

		return statusErr.Error()
	}

	return err.Error()
}
