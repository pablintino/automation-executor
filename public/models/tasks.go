package models

type TaskDefinition struct {
	Name              string `json:"name"`
	Type              string `json:"type"`
	Repositories      []string
	ArtifactsPatterns []string
	Image             string
}

type AnsibleTaskDefinition struct {
	TaskDefinition
	Inventory       map[string]interface{}
	InventoryPath   string
	Variables       []string
	VariablesPaths  []string
	Playbook        string
	OutputsPatterns []string
}
