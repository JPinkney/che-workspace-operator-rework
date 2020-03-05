package adaptor

import (
	"fmt"
	"github.com/che-incubator/che-workspace-operator/pkg/apis/workspace/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func adaptResourcesFromString(memLimit string) (corev1.ResourceRequirements, error) {
	memLimitQuantity, err := resource.ParseQuantity(memLimit)
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}
	resources := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{
			corev1.ResourceMemory: memLimitQuantity,
		},
		Requests: nil,
	}

	return resources, nil
}

func SortComponentsByType(components []v1alpha1.ComponentSpec) (dockerimage, plugin []v1alpha1.ComponentSpec, err error) {
	for _, component := range components {
		switch component.Type {
		case v1alpha1.Dockerimage:
			dockerimage = append(dockerimage, component)
		case v1alpha1.CheEditor, v1alpha1.ChePlugin:
			plugin = append(plugin, component)
		default:
			return nil, nil, fmt.Errorf("unsupported component type encountered: %s", component.Type)
		}
	}
	return
}
