package types

type ProjectType string

type ProjectConfig struct {
	Type           ProjectType
	ID             string
	Desc           string
	Owner          string            `json:"owner"`
	Labels         map[string]string `json:"labels"`
	SourceRepo     string            `json:"sourceRepo,omitempty"`
	ReleaseGitRepo string            `json:"relaseRepo"`
	DeployDir      string            `json:"deployDir,omitempty"`
}

type ProjectDetail interface {
	GetProjectConfig() ProjectConfig
}
