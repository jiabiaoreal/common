package java

import (
	"we.com/jiabiao/common/types"
)

const (
	projectType types.ProjectType = "java"
)

type Project struct {
	types.ProjectConfig

	APIVersion   string `json:"apiVersion"`
	RegistryBase string `json:"serverZKBase,omitempty"`
}
