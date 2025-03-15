package utils

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"ignite-api/internal/config"
	"ignite-api/internal/logger"
	"log"
	"os"
	"os/exec"

	"gopkg.in/yaml.v2"
)

// WriteTempFile writes a temporary file with the given data and pattern
func WriteTempFile(data []byte, pattern string) (string, error) {
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

// StoreProvisionInfo stores the provisioned VM information in a CSV file
func StoreProvisionInfo(nodeName, nodeUID, masterIP, nodeType, token string) error {
	file, err := os.OpenFile("provisioned_vms.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	if fileInfo.Size() == 0 {
		headers := []string{"NodeName", "NodeUID", "MasterIP", "NodeType", "Token"}
		if err := writer.Write(headers); err != nil {
			return fmt.Errorf("failed to write headers to CSV file: %w", err)
		}
	}

	record := []string{nodeName, nodeUID, masterIP, nodeType, token}
	if err := writer.Write(record); err != nil {
		return fmt.Errorf("failed to write to CSV file: %w", err)
	}

	return nil
}

// CreateTempConfigFile creates a temporary config file from the Config object
func CreateTempConfigFile(config config.Config) (string, error) {
	configContent, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("Failed to marshal config: %w", err)
	}
	return WriteTempFile(configContent, "config-*.json")
}

// CreateTempManifestFile creates a temporary manifest file from the Manifest object
func CreateTempManifestFile(manifest config.Manifest) (string, error) {
	manifestYAML, err := yaml.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("Error marshaling manifest: %w", err)
	}
	return WriteTempFile(manifestYAML, "ignite-config-*.yaml")
}

// RunIgnite executes the ignite command with the given manifest file
func RunIgnite(manifestFileName, nodeName string) error {
	cmd := exec.Command("sudo", "ignite", "run", "--config", manifestFileName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	logger.Info("Provisioning VM: %s", nodeName)
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to run ignite: %v\nStdout: %s\nStderr: %s",
			err, stdout.String(), stderr.String())
		return fmt.Errorf("Error running ignite: %v\nStdout: %s\nStderr: %s",
			err, stdout.String(), stderr.String())
	}
	return nil
}

// GetMasterIP executes `ignite ps` and parses the output to find the IP of the master node
func GetMasterIP(nodeName string) (string, error) {
	cmd := exec.Command("sudo", "ignite", "ps")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error running ignite ps: %v\nStderr: %s", err, stderr.String())
	}

	lines := bytes.Split(stdout.Bytes(), []byte("\n"))
	for _, line := range lines {
		if bytes.Contains(line, []byte(nodeName)) {
			fields := bytes.Fields(line)
			if len(fields) >= 12 {
				return string(fields[12]), nil
			}
		}
	}

	return "", fmt.Errorf("IP address for node '%s' not found", nodeName)
}

// RunIgniteCommand runs an ignite command with the given action and VM name
func RunIgniteCommand(action, vmName string) error {
	cmd := exec.Command("sudo", "ignite", "vm", action, vmName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Failed to %s VM: %v", action, err)
	}
	return nil
}

// ValidateTokenAndMasterIP checks if the token and master IP match any existing records
func ValidateTokenAndMasterIP(token, masterIP string) bool {
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
			if len(record) >= 5 && record[4] == token {
				return true
			}
		}
	}

	return false
}
