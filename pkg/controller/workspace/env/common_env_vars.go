package env

import (
	"github.com/che-incubator/che-workspace-operator/pkg/config"
	corev1 "k8s.io/api/core/v1"
)

func CommonEnvironmentVariables(workspaceName, workspaceId, namespace string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "CHE_MACHINE_TOKEN",
		},
		{
			Name:  "CHE_PROJECTS_ROOT",
			Value: config.DefaultProjectsSourcesRoot,
		},
		{
			Name:  "CHE_API",
			Value: config.DefaultApiEndpoint,
		},
		{
			Name:  "CHE_API_INTERNAL",
			Value: config.DefaultApiEndpoint,
		},
		{
			Name:  "CHE_API_EXTERNAL",
			Value: config.DefaultApiEndpoint,
		},
		{
			Name:  "CHE_WORKSPACE_NAME",
			Value: workspaceName,
		},
		{
			Name:  "CHE_WORKSPACE_ID",
			Value: workspaceId,
		},
		{
			Name:  "CHE_AUTH_ENABLED",
			Value: config.AuthEnabled,
		},
		{
			Name:  "CHE_WORKSPACE_NAMESPACE",
			Value: namespace,
		},
		{ //TODO
			Name: "LOG_LEVEL",
			Value: "debug",
		},
	}
}