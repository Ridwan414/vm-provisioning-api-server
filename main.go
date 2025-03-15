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
	app.Post("/master/provision", masterProvisionHandler)
	app.Post("/worker/provision", workerProvisionHandler)
	app.Delete("/master/vm/:name", deleteVMHandler)

	// Start server
	port := "5050"
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

// masterProvisionHandler handles the provision request for master nodes
func masterProvisionHandler(c *fiber.Ctx) error {
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
	log.Printf("Received provision request: %+v\n", request)

	// Validate required fields
	if request.NodeName == "" || request.NodeUID == "" {
		log.Printf("NodeName and NodeUID are required fields\n")
		return c.Status(400).JSON(ProvisionResponse{
			Success: false,
			Error:   "NodeName and NodeUID are required fields",
		})
	}

	// Create config data in memory
	config := Config{
		Name:     request.NodeName,
		UID:      request.NodeUID,
		NodeType: request.NodeType,
		Token:    request.Token,
		MasterIP: request.MasterIP,
	}

	// Create manifest data in memory
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

	// Create config.json as a copyFile entry (using temp file)
	configContent, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal config: %v\n", err)
		return c.Status(500).JSON(ProvisionResponse{
			Success: false,
			Error:   "Failed to marshal config: " + err.Error(),
		})
	}

	configFileName, err := writeTempFile(configContent, "config-*.json")
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
	manifestYAML, err := yaml.Marshal(manifest)
	if err != nil {
		log.Printf("Error marshaling manifest: %v\n", err)
		return c.Status(500).JSON(ProvisionResponse{
			Success: false,
			Error:   "Error marshaling manifest: " + err.Error(),
		})
	}

	manifestFileName, err := writeTempFile(manifestYAML, "ignite-config-*.yaml")
	if err != nil {
		log.Printf("%v\n", err)
		return c.Status(500).JSON(ProvisionResponse{
			Success: false,
			Error:   err.Error(),
		})
	}
	defer os.Remove(manifestFileName)

	// Execute ignite using the temporary file
	cmd := exec.Command("sudo", "ignite", "run", "--config", manifestFileName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("Provisioning VM: %s\n", request.NodeName)
	if err := cmd.Run(); err != nil {
		errMsg := fmt.Sprint("Error running ignite: ", err, "\nStdout: ", stdout.String(), "\nStderr: ", stderr.String())
		log.Print(errMsg)
		return c.Status(500).JSON(ProvisionResponse{
			Success: false,
			Error:   errMsg,
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

	// Return success response
	log.Printf("VM '%s' successfully provisioned with IP %s\n", request.NodeName, masterIP)

	// Store provisioned VM info in CSV
	if err := storeProvisionInfo(request.NodeName, request.NodeUID, masterIP, request.NodeType, request.Token); err != nil {
		log.Printf("Failed to store provision info: %v\n", err)
		return c.Status(500).JSON(ProvisionResponse{
			Success: false,
			Error:   "Failed to store provision info: " + err.Error(),
		})
	}

	return c.JSON(ProvisionResponse{
		Success:  true,
		Message:  fmt.Sprintf("VM '%s' successfully provisioned", request.NodeName),
		NodeID:   request.NodeUID,
		MasterIP: masterIP,
	})
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
			// for i, field := range fields {
			// 	log.Printf("Field %d: %s\n", i, string(field))
			// }
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
	stopCmd := exec.Command("sudo", "ignite", "vm", "stop", vmName)
	if err := stopCmd.Run(); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("Failed to stop VM: %v", err),
		})
	}

	// Remove the VM
	rmCmd := exec.Command("sudo", "ignite", "vm", "rm", vmName)
	if err := rmCmd.Run(); err != nil {
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

// workerProvisionHandler handles the provision request for worker nodes
func workerProvisionHandler(c *fiber.Ctx) error {
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
	log.Printf("Received worker provision request: %+v\n", request)

	// Validate required fields
	if request.NodeName == "" || request.NodeUID == "" || request.MasterIP == "" || request.NodeType != "worker" {
		log.Printf("NodeName, NodeUID, MasterIP, and NodeType 'worker' are required fields\n")
		return c.Status(400).JSON(ProvisionResponse{
			Success: false,
			Error:   "NodeName, NodeUID, MasterIP, and NodeType 'worker' are required fields",
		})
	}

	// Check if the token and master IP match from the CSV
	if !validateTokenAndMasterIP(request.Token, request.MasterIP) {
		log.Printf("Token and MasterIP do not match any existing records\n")
		return c.Status(400).JSON(ProvisionResponse{
			Success: false,
			Error:   "Token and MasterIP do not match any existing records",
		})
	}

	// Create config data in memory
	config := Config{
		Name:     request.NodeName,
		UID:      request.NodeUID,
		NodeType: request.NodeType,
		Token:    request.Token,
		MasterIP: request.MasterIP,
	}

	// Create manifest data in memory
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

	// Create config.json as a copyFile entry (using temp file)
	configContent, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal config: %v\n", err)
		return c.Status(500).JSON(ProvisionResponse{
			Success: false,
			Error:   "Failed to marshal config: " + err.Error(),
		})
	}

	configFileName, err := writeTempFile(configContent, "config-*.json")
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
	manifestYAML, err := yaml.Marshal(manifest)
	if err != nil {
		log.Printf("Error marshaling manifest: %v\n", err)
		return c.Status(500).JSON(ProvisionResponse{
			Success: false,
			Error:   "Error marshaling manifest: " + err.Error(),
		})
	}

	manifestFileName, err := writeTempFile(manifestYAML, "ignite-config-*.yaml")
	if err != nil {
		log.Printf("%v\n", err)
		return c.Status(500).JSON(ProvisionResponse{
			Success: false,
			Error:   err.Error(),
		})
	}
	defer os.Remove(manifestFileName)

	// Execute ignite using the temporary file
	cmd := exec.Command("sudo", "ignite", "run", "--config", manifestFileName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("Provisioning worker VM: %s\n", request.NodeName)
	if err := cmd.Run(); err != nil {
		errMsg := fmt.Sprint("Error running ignite: ", err, "\nStdout: ", stdout.String(), "\nStderr: ", stderr.String())
		log.Print(errMsg)
		return c.Status(500).JSON(ProvisionResponse{
			Success: false,
			Error:   errMsg,
		})
	}

	// Return success response
	log.Printf("Worker VM '%s' successfully provisioned\n", request.NodeName)

	return c.JSON(ProvisionResponse{
		Success: true,
		Message: fmt.Sprintf("Worker VM '%s' successfully provisioned", request.NodeName),
		NodeID:  request.NodeUID,
	})
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
			// Assuming the token is stored in the 4th column
			if len(record) >= 4 && record[3] == token {
				return true
			}
		}
	}

	return false
}
