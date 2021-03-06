package v1alpha1

type ComponentDescription struct {
	Name              string            `json:"name"`
	PodAdditions      PodAdditions      `json:"podAdditions"`
	ComponentMetadata ComponentMetadata `json:"componentMetadata"`
}

type ComponentMetadata struct {
	// Containers is a map of container names to ContainerDescriptions. Field is serialized into workspace status "additionalFields"
	// and consumed by che-rest-apis
	Containers map[string]ContainerDescription `json:"containers,omitempty"`
	// ContributedRuntimeCommands represent the devfile commands available in the current workspace. They are serialized into the
	// workspace status "additionalFields" and consumed by che-rest-apis.
	ContributedRuntimeCommands []CheWorkspaceCommand `json:"contributedRuntimeCommands,omitempty"`
	// Endpoints stores the workspace endpoints defined by the component
	Endpoints []Endpoint `json:"endpoints,omitempty"`
}

// ContainerDescription stores metadata about workspace containers. This is used to provide information
// to Theia via the che-rest-apis container.
type ContainerDescription struct {
	// Attributes stores the Che-specific metadata about a component, e.g. a plugin's ID, memoryLimit from devfile, etc.
	Attributes map[string]string `json:"attributes,omitempty"`
	// Ports stores the list of ports exposed by this container.
	Ports []int `json:"ports,omitempty"`
}

type CheWorkspaceCommand struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	CommandLine string            `json:"commandLine"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}
