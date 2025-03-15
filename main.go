package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/gofiber/fiber/v2"
	"gopkg.in/yaml.v2"
)

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

// ProvisionRequest represents the API request payload
type ProvisionRequest struct {
	NodeName  string `json:"nodeName"`
	NodeUID   string `json:"nodeUid"`
	NodeType  string `json:"nodeType"`
	Token     string `json:"token"`
	MasterIP  string `json:"masterIP"`
	CPUs      int    `json:"cpus"`
	DiskSize  string `json:"diskSize"`
	Memory    string `json:"memory"`
	ImageOCI  string `json:"imageOci"`
	EnableSSH bool   `json:"enableSsh"`
}

// ProvisionResponse represents the API response
type ProvisionResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	NodeID   string `json:"nodeId,omitempty"`
	Error    string `json:"error,omitempty"`
	MasterIP string `json:"masterIP,omitempty"`
}

func main() {
	app := fiber.New()

	// Define API endpoints
	app.Get("/health", healthHandler)
	app.Post("/master/provision", provisionHandler("master"))
	app.Post("/worker/provision", provisionHandler("worker"))
	app.Delete("/vm/:name", deleteVMHandler)

	// Start server
	port := "5090"
	log.Printf("Starting Ignite API server on port %s...\n", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v\n", err)
	}
}

// healthHandler handles the health check request
func healthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "ok",
		"service": "ignite-provision-api",
	})
}

// writeTempFile writes a temporary file with the given data and pattern
func writeTempFile(data []byte, pattern string) (string, error) {
	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("error creating temp file: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("error writing to temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("error closing temp file: %w", err)
	}

	return tmpFile.Name(), nil
}

// storeProvisionInfo stores the provisioned VM information in a CSV file
func storeProvisionInfo(nodeName, nodeUID, masterIP, nodeType, token string) error {
	file, err := os.OpenFile("provisioned_vms.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Check if the file is empty to write headers
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	if fileInfo.Size() == 0 {
		// Write headers if the file is empty
		headers := []string{"NodeName", "NodeUID", "MasterIP", "NodeType", "Token"}
		if err := writer.Write(headers); err != nil {
			return fmt.Errorf("failed to write headers to CSV file: %w", err)
		}
	}

	// Write the record
	record := []string{nodeName, nodeUID, masterIP, nodeType, token}
	if err := writer.Write(record); err != nil {
		return fmt.Errorf("failed to write to CSV file: %w", err)
	}

	return nil
}

// provisionHandler handles the provision request for nodes
func provisionHandler(nodeType string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Parse request body
		request := new(ProvisionRequest)
		if err := c.BodyParser(request); err != nil {
			log.Printf("Invalid request format: %v\n", err)
			return c.Status(400).JSON(ProvisionResponse{
				Success: false,
				Error:   "Invalid request format: " + err.Error(),
			})
		}

		// Log the request body for debugging
		log.Printf("Received %s provision request: %+v\n", nodeType, request)

		// Validate required fields
		if err := validateProvisionRequest(request, nodeType); err != nil {
			log.Printf("%v\n", err)
			return c.Status(400).JSON(ProvisionResponse{
				Success: false,
				Error:   err.Error(),
			})
		}

		// Create config and manifest data in memory
		config := createConfig(request)
		manifest := createManifest(request)

		// Create config.json as a copyFile entry (using temp file)
		configFileName, err := createTempConfigFile(config)
		if err != nil {
			log.Printf("%v\n", err)
			return c.Status(500).JSON(ProvisionResponse{
				Success: false,
				Error:   err.Error(),
			})
		}
		defer os.Remove(configFileName)

		manifest.Spec.CopyFiles = []struct {
			HostPath string `yaml:"hostPath"`
			VMPath   string `yaml:"vmPath"`
		}{
			{
				HostPath: configFileName,
				VMPath:   "/root/config.json",
			},
		}

		// Convert manifest to YAML
		manifestFileName, err := createTempManifestFile(manifest)
		if err != nil {
			log.Printf("%v\n", err)
			return c.Status(500).JSON(ProvisionResponse{
				Success: false,
				Error:   err.Error(),
			})
		}
		defer os.Remove(manifestFileName)

		// Execute ignite using the temporary file
		if err := runIgnite(manifestFileName, request.NodeName); err != nil {
			log.Printf("%v\n", err)
			return c.Status(500).JSON(ProvisionResponse{
				Success: false,
				Error:   err.Error(),
			})
		}

		// Get the master IP by running `ignite ps`
		masterIP, err := getMasterIP(request.NodeName)
		if err != nil {
			log.Printf("Failed to get master IP: %v\n", err)
			return c.Status(500).JSON(ProvisionResponse{
				Success: false,
				Error:   "Failed to get master IP: " + err.Error(),
			})
		}

		// Store provisioned VM info in CSV
		if err := storeProvisionInfo(request.NodeName, request.NodeUID, masterIP, request.NodeType, request.Token); err != nil {
			log.Printf("Failed to store provision info: %v\n", err)
			return c.Status(500).JSON(ProvisionResponse{
				Success: false,
				Error:   "Failed to store provision info: " + err.Error(),
			})
		}

		// Return success response
		log.Printf("VM '%s' successfully provisioned with IP %s\n", request.NodeName, masterIP)
		return c.JSON(ProvisionResponse{
			Success:  true,
			Message:  fmt.Sprintf("VM '%s' successfully provisioned", request.NodeName),
			NodeID:   request.NodeUID,
			MasterIP: masterIP,
		})
	}
}

// validateProvisionRequest validates the provision request based on node type
func validateProvisionRequest(request *ProvisionRequest, nodeType string) error {
	if request.NodeName == "" || request.NodeUID == "" {
		return fmt.Errorf("NodeName and NodeUID are required fields")
	}
	if nodeType == "worker" && (request.MasterIP == "" || request.NodeType != "worker") {
		return fmt.Errorf("NodeName, NodeUID, MasterIP, and NodeType 'worker' are required fields")
	}
	if nodeType == "worker" && !validateTokenAndMasterIP(request.Token, request.MasterIP) {
		return fmt.Errorf("Token and MasterIP do not match any existing records")
	}
	return nil
}

// createConfig creates a Config object from the provision request
func createConfig(request *ProvisionRequest) Config {
	return Config{
		Name:     request.NodeName,
		UID:      request.NodeUID,
		NodeType: request.NodeType,
		Token:    request.Token,
		MasterIP: request.MasterIP,
	}
}

// createManifest creates a Manifest object from the provision request
func createManifest(request *ProvisionRequest) Manifest {
	manifest := Manifest{
		APIVersion: "ignite.weave.works/v1alpha4",
		Kind:       "VM",
	}
	manifest.Metadata.Name = request.NodeName
	manifest.Metadata.UID = request.NodeUID

	// Set defaults if not provided
	cpus := request.CPUs
	if cpus <= 0 {
		cpus = 2
	}
	diskSize := request.DiskSize
	if diskSize == "" {
		diskSize = "3GB"
	}
	memory := request.Memory
	if memory == "" {
		memory = "1GB"
	}
	imageOCI := request.ImageOCI
	if imageOCI == "" {
		imageOCI = "shajalahamedcse/only-k3-go:v1.0.10"
	}

	// Log the imageOCI value for debugging
	log.Printf("Using image OCI: %s\n", imageOCI)

	manifest.Spec.Image = map[string]string{"oci": imageOCI}
	manifest.Spec.CPUs = cpus
	manifest.Spec.DiskSize = diskSize
	manifest.Spec.Memory = memory
	manifest.Spec.SSH = request.EnableSSH

	return manifest
}

// createTempConfigFile creates a temporary config file from the Config object
func createTempConfigFile(config Config) (string, error) {
	configContent, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("Failed to marshal config: %w", err)
	}
	return writeTempFile(configContent, "config-*.json")
}

// createTempManifestFile creates a temporary manifest file from the Manifest object
func createTempManifestFile(manifest Manifest) (string, error) {
	manifestYAML, err := yaml.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("Error marshaling manifest: %w", err)
	}
	return writeTempFile(manifestYAML, "ignite-config-*.yaml")
}

// runIgnite executes the ignite command with the given manifest file
func runIgnite(manifestFileName, nodeName string) error {
	cmd := exec.Command("sudo", "ignite", "run", "--config", manifestFileName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("Provisioning VM: %s\n", nodeName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Error running ignite: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
	}
	return nil
}

// getMasterIP executes `ignite ps` and parses the output to find the IP of the master node
func getMasterIP(nodeName string) (string, error) {
	cmd := exec.Command("sudo", "ignite", "ps")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error running ignite ps: %v\nStderr: %s", err, stderr.String())
	}

	// Parse the output to find the IP address
	lines := bytes.Split(stdout.Bytes(), []byte("\n"))
	for _, line := range lines {
		if bytes.Contains(line, []byte(nodeName)) {
			// Assuming the IP is the 8th column in the output
			fields := bytes.Fields(line)
			if len(fields) >= 12 {
				return string(fields[12]), nil
			}
		}
	}

	return "", fmt.Errorf("IP address for node '%s' not found", nodeName)
}

// deleteVMHandler handles the deletion of a VM
func deleteVMHandler(c *fiber.Ctx) error {
	vmName := c.Params("name")
	if vmName == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "VM name is required",
		})
	}

	// Stop the VM
	if err := runIgniteCommand("stop", vmName); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("Failed to stop VM: %v", err),
		})
	}

	// Remove the VM
	if err := runIgniteCommand("rm", vmName); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("Failed to remove VM: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("VM '%s' successfully deleted", vmName),
	})
}

// runIgniteCommand runs an ignite command with the given action and VM name
func runIgniteCommand(action, vmName string) error {
	cmd := exec.Command("sudo", "ignite", "vm", action, vmName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Failed to %s VM: %v", action, err)
	}
	return nil
}

// validateTokenAndMasterIP checks if the token and master IP match any existing records in the CSV
func validateTokenAndMasterIP(token, masterIP string) bool {
	file, err := os.Open("provisioned_vms.csv")
	if err != nil {
		log.Printf("Failed to open CSV file: %v\n", err)
		return false
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		log.Printf("Failed to read CSV file: %v\n", err)
		return false
	}

	for _, record := range records {
		if len(record) >= 3 && record[2] == masterIP {
			// Assuming the token is stored in the 5th column
			if len(record) >= 5 && record[4] == token {
				return true
			}
		}
	}

	return false
}
