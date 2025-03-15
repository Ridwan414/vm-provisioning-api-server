package config

// Config represents the master config structure
type Config struct {
	Name     string `json:"name"`
	UID      string `json:"uid"`
	NodeType string `json:"nodeType"`
	Token    string `json:"token"`
	MasterIP string `json:"masterIP"`
}

// Manifest represents the VM manifest structure
type Manifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
		UID  string `yaml:"uid"`
	} `yaml:"metadata"`
	Spec struct {
		Image     map[string]string `yaml:"image"`
		CPUs      int               `yaml:"cpus"`
		DiskSize  string            `yaml:"diskSize"`
		Memory    string            `yaml:"memory"`
		CopyFiles []struct {
			HostPath string `yaml:"hostPath"`
			VMPath   string `yaml:"vmPath"`
		} `yaml:"copyFiles"`
		SSH bool `yaml:"ssh"`
	} `yaml:"spec"`
}
