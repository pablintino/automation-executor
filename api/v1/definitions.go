package models

type RepoSourceDefinition struct {
	URL       string `json:"url"`
	SecretRef string `json:"secret-ref"`
}

type EnvDefinition struct {
	Type         string `json:"type"`
	Repositories []RepoSourceDefinition
}
