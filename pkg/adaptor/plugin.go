package adaptor

import (
	"fmt"
	"github.com/che-incubator/che-workspace-operator/pkg/apis/workspace/v1alpha1"
	"github.com/che-incubator/che-workspace-operator/pkg/common"
	"github.com/che-incubator/che-workspace-operator/pkg/config"
	metadataBroker "github.com/eclipse/che-plugin-broker/brokers/metadata"
	brokerModel "github.com/eclipse/che-plugin-broker/model"
	"github.com/eclipse/che-plugin-broker/utils"
	corev1 "k8s.io/api/core/v1"
	"strconv"
	"strings"
)

func AdaptPluginComponents(workspaceId, namespace string, devfileComponents []v1alpha1.ComponentSpec) ([]v1alpha1.ComponentDescription, *corev1.ConfigMap, error) {
	var components []v1alpha1.ComponentDescription

	broker := metadataBroker.NewBroker(true)

	metas, aliases, err := getMetasForComponents(devfileComponents)
	if err != nil {
		return nil, nil, err
	}
	plugins, err := broker.ProcessPlugins(metas)
	if err != nil {
		return nil, nil, err
	}

	for _, plugin := range plugins {
		component, err := adaptChePluginToComponent(workspaceId, plugin)
		if err != nil {
			return nil, nil, err
		}
		if aliases[plugin.ID] != "" {
			component.Name = aliases[plugin.ID]
		}

		components = append(components, component)
	}

	var artifactsBrokerCM *corev1.ConfigMap
	if isArtifactsBrokerNecessary(metas) {
		artifactsBrokerComponent, configMap, err := getArtifactsBrokerComponent(workspaceId, namespace, devfileComponents)
		if err != nil {
			return nil, nil, err
		}
		components = append(components, *artifactsBrokerComponent)
		artifactsBrokerCM = configMap
	}

	return components, artifactsBrokerCM, nil
}

func adaptChePluginToComponent(workspaceId string, plugin brokerModel.ChePlugin) (v1alpha1.ComponentDescription, error) {
	var containers []corev1.Container
	containerDescriptions := map[string]v1alpha1.ContainerDescription{}
	for _, pluginContainer := range plugin.Containers {
		container, containerDescription, err := convertPluginContainer(workspaceId, plugin.ID, pluginContainer)
		if err != nil {
			return v1alpha1.ComponentDescription{}, err
		}
		containers = append(containers, container)
		containerDescriptions[container.Name] = containerDescription
	}
	var initContainers []corev1.Container
	for _, pluginInitContainer := range plugin.InitContainers {
		container, _, err := convertPluginContainer(workspaceId, plugin.ID, pluginInitContainer)
		if err != nil {
			return v1alpha1.ComponentDescription{}, err
		}
		initContainers = append(initContainers, container)
	}

	componentName := plugin.Name
	if len(plugin.Containers) > 0 {
		componentName = plugin.Containers[0].Name
	}
	component := v1alpha1.ComponentDescription{
		Name: componentName,
		PodAdditions: v1alpha1.PodAdditions{
			Containers:     containers,
			InitContainers: initContainers,
		},
		ComponentMetadata: v1alpha1.ComponentMetadata{
			Containers:                 containerDescriptions,
			ContributedRuntimeCommands: GetPluginComponentCommands(plugin), // TODO: Can regular commands apply to plugins in devfile spec?
			Endpoints:                  createEndpointsFromPlugin(plugin),
		},
	}

	return component, nil
}

func createEndpointsFromPlugin(plugin brokerModel.ChePlugin) []v1alpha1.Endpoint {
	var endpoints []v1alpha1.Endpoint

	for _, pluginEndpoint := range plugin.Endpoints {
		attributes := map[v1alpha1.EndpointAttribute]string{}
		// Default value of http for protocol, may be overwritten by pluginEndpoint attributes
		attributes[v1alpha1.PROTOCOL_ENDPOINT_ATTRIBUTE] = "http"
		attributes[v1alpha1.PUBLIC_ENDPOINT_ATTRIBUTE] = strconv.FormatBool(pluginEndpoint.Public)
		for key, val := range pluginEndpoint.Attributes {
			attributes[v1alpha1.EndpointAttribute(key)] = val
		}
		endpoints = append(endpoints, v1alpha1.Endpoint{
			Name:       common.EndpointName(pluginEndpoint.Name),
			Port:       int64(pluginEndpoint.TargetPort),
			Attributes: attributes,
		})
	}

	return endpoints
}

func convertPluginContainer(workspaceId, pluginID string, brokerContainer brokerModel.Container) (corev1.Container, v1alpha1.ContainerDescription, error) {
	memorylimit := brokerContainer.MemoryLimit
	if memorylimit == "" {
		memorylimit = config.SidecarDefaultMemoryLimit
	}
	containerResources, err := adaptResourcesFromString(memorylimit)
	if err != nil {
		return corev1.Container{}, v1alpha1.ContainerDescription{}, err
	}

	var env []corev1.EnvVar
	for _, brokerEnv := range brokerContainer.Env {
		env = append(env, corev1.EnvVar{
			Name:  brokerEnv.Name,
			Value: brokerEnv.Value,
		})
	}

	var containerPorts []corev1.ContainerPort
	var portInts []int
	for _, brokerPort := range brokerContainer.Ports {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			ContainerPort: int32(brokerPort.ExposedPort),
			Protocol:      corev1.ProtocolTCP,
		})
		portInts = append(portInts, brokerPort.ExposedPort)
	}

	container := corev1.Container{
		Name:            brokerContainer.Name,
		Image:           brokerContainer.Image,
		Command:         brokerContainer.Command,
		Args:            brokerContainer.Args,
		Ports:           containerPorts,
		Env:             env,
		Resources:       containerResources,
		VolumeMounts:    adaptVolumeMountsFromBroker(workspaceId, brokerContainer),
		ImagePullPolicy: corev1.PullAlways,
	}

	containerDescription := v1alpha1.ContainerDescription{
		Attributes: map[string]string{
			config.RestApisContainerSourceAttribute: config.RestApisRecipeSourceToolAttribute,
			config.RestApisPluginMachineAttribute:   pluginID,
		},
		Ports: portInts,
	}

	return container, containerDescription, nil
}

func adaptVolumeMountsFromBroker(workspaceId string, brokerContainer brokerModel.Container) []corev1.VolumeMount {
	var volumeMounts []corev1.VolumeMount
	volumeName := config.ControllerCfg.GetWorkspacePVCName()

	// TODO: Handle ephemeral
	for _, brokerVolume := range brokerContainer.Volumes {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			SubPath:   fmt.Sprintf("%s/%s/", workspaceId, brokerVolume.Name),
			MountPath: brokerVolume.MountPath,
		})
	}
	if brokerContainer.MountSources {
		volumeMounts = append(volumeMounts, GetProjectSourcesVolumeMount(workspaceId))
	}

	return volumeMounts
}

func getMetasForComponents(components []v1alpha1.ComponentSpec) (metas []brokerModel.PluginMeta, aliases map[string]string, err error) {
	defaultRegistry := config.ControllerCfg.GetPluginRegistry()
	ioUtils := utils.New()
	aliases = map[string]string{}
	for _, component := range components {
		if component.Type != v1alpha1.ChePlugin && component.Type != v1alpha1.CheEditor {
			return nil, nil, fmt.Errorf("cannot adapt non-plugin or editor type component %s in plugin adaptor", component.Type)
		}
		fqn := getPluginFQN(component)
		meta, err := utils.GetPluginMeta(fqn, defaultRegistry, ioUtils)
		if err != nil {
			return nil, nil, err
		}
		metas = append(metas, *meta)
		aliases[meta.ID] = component.Alias
	}
	err = utils.ResolveRelativeExtensionPaths(metas, defaultRegistry)
	if err != nil {
		return nil, nil, err
	}
	return metas, aliases, nil
}

func getPluginFQN(component v1alpha1.ComponentSpec) brokerModel.PluginFQN {
	var pluginFQN brokerModel.PluginFQN
	registryAndID := strings.Split(component.Id, "#")
	if len(registryAndID) == 2 {
		pluginFQN.Registry = registryAndID[0]
		pluginFQN.ID = registryAndID[1]
	} else if len(registryAndID) == 1 {
		pluginFQN.ID = registryAndID[0]
	}
	pluginFQN.Reference = component.Reference
	return pluginFQN
}

func GetPluginComponentCommands(plugin brokerModel.ChePlugin) []v1alpha1.CheWorkspaceCommand {
	var commands []v1alpha1.CheWorkspaceCommand

	for _, pluginContainer := range plugin.Containers {
		for _, pluginCommand := range pluginContainer.Commands {
			command := v1alpha1.CheWorkspaceCommand{
				Name:        pluginCommand.Name,
				CommandLine: strings.Join(pluginCommand.Command, " "),
				Type:        "custom",
				Attributes: map[string]string{
					config.CommandWorkingDirectoryAttribute: pluginCommand.WorkingDir, // TODO: Env Var substitution?
					config.CommandMachineNameAttribute:      pluginContainer.Name,
				},
			}
			commands = append(commands, command)
		}
	}

	return commands
}
