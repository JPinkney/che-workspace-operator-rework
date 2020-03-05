//
// Copyright (c) 2019-2020 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package server

import (
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	"fmt"

	. "github.com/che-incubator/che-workspace-operator/pkg/controller/workspace/config"
	. "github.com/che-incubator/che-workspace-operator/pkg/controller/workspace/model"
)

func AddCheRestApis(wkspCtx WorkspaceContext, podSpec *corev1.PodSpec) ([]runtime.Object, string, error) {
	cheRestApisPort := 9999
	containerName := "che-rest-apis"
	podSpec.Containers = append(podSpec.Containers, corev1.Container{
		Image:           ControllerCfg.GetCheRestApisDockerImage(),
		ImagePullPolicy: corev1.PullPolicy(ControllerCfg.GetSidecarPullPolicy()),
		Name:            containerName,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: int32(cheRestApisPort),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "CHE_WORKSPACE_NAME",
				Value: wkspCtx.WorkspaceName,
			},
			{
				Name:  "CHE_WORKSPACE_ID",
				Value: wkspCtx.WorkspaceId,
			},
			{
				Name:  "CHE_WORKSPACE_NAMESPACE",
				Value: wkspCtx.Namespace,
			},
		},
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	})

	//serviceName, servicePort := containerName, specutils.ServicePortName(cheRestApisPort)
	//ingressName := specutils.IngressName(serviceName, int64(cheRestApisPort))
	//ingressHost := specutils.IngressHostname(serviceName, wkspCtx.Namespace, ControllerCfg.GetIngressGlobalDomain(), int64(cheRestApisPort))
	serviceName, servicePort := containerName, "che-rest-apis"
	ingressName := "che-rest-apis"
	ingressHost := "todo"

	ingressUrl := "http://" + ingressHost + "/api"

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceName,
			Namespace:   wkspCtx.Namespace,
			Annotations: map[string]string{},
			Labels: map[string]string{
				WorkspaceIDLabel: wkspCtx.WorkspaceId,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				CheOriginalNameLabel: CheOriginalName,
				WorkspaceIDLabel:     wkspCtx.WorkspaceId,
			},
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "todo",
					Protocol:   ServicePortProtocol,
					Port:       int32(cheRestApisPort),
					TargetPort: intstr.FromInt(cheRestApisPort),
				},
			},
		},
	}
	ingress := extensionsv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("ingress-%s-%s", wkspCtx.WorkspaceId, containerName),
			Namespace: wkspCtx.Namespace,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class":                "nginx",
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
				"nginx.ingress.kubernetes.io/ssl-redirect":   "false",
				"org.eclipse.che.machine.name":               containerName,
			},
			Labels: map[string]string{
				CheOriginalNameLabel: ingressName,
				WorkspaceIDLabel:     wkspCtx.WorkspaceId,
			},
		},
		Spec: extensionsv1beta1.IngressSpec{
			Rules: []extensionsv1beta1.IngressRule{
				{
					IngressRuleValue: extensionsv1beta1.IngressRuleValue{
						HTTP: &extensionsv1beta1.HTTPIngressRuleValue{
							Paths: []extensionsv1beta1.HTTPIngressPath{
								{
									Backend: extensionsv1beta1.IngressBackend{
										ServiceName: serviceName,
										ServicePort: intstr.FromString(servicePort),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	ingress.Spec.Rules[0].Host = ingressHost

	return []runtime.Object{&service, &ingress}, ingressUrl, nil
}
