package api

import (
	"fmt"
	"ignite-api/internal/config"
	"ignite-api/internal/models"
	"ignite-api/internal/utils"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
)

// HealthHandler handles the health check request
func HealthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "ok",
		"service": "ignite-provision-api",
	})
}

// ProvisionHandler handles the provision request for nodes
func ProvisionHandler(nodeType string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		request := new(models.ProvisionRequest)
		if err := c.BodyParser(request); err != nil {
			log.Printf("Invalid request format: %v\n", err)
			return c.Status(400).JSON(models.ProvisionResponse{
				Success: false,
				Error:   "Invalid request format: " + err.Error(),
			})
		}

		log.Printf("Received %s provision request: %+v\n", nodeType, request)

		if err := validateProvisionRequest(request, nodeType); err != nil {
			log.Printf("%v\n", err)
			return c.Status(400).JSON(models.ProvisionResponse{
				Success: false,
				Error:   err.Error(),
			})
		}

		cfg := createConfig(request)
		manifest := createManifest(request)

		configFileName, err := utils.CreateTempConfigFile(cfg)
		if err != nil {
			log.Printf("%v\n", err)
			return c.Status(500).JSON(models.ProvisionResponse{
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

		manifestFileName, err := utils.CreateTempManifestFile(manifest)
		if err != nil {
			log.Printf("%v\n", err)
			return c.Status(500).JSON(models.ProvisionResponse{
				Success: false,
				Error:   err.Error(),
			})
		}
		defer os.Remove(manifestFileName)

		if err := utils.RunIgnite(manifestFileName, request.NodeName); err != nil {
			log.Printf("%v\n", err)
			return c.Status(500).JSON(models.ProvisionResponse{
				Success: false,
				Error:   err.Error(),
			})
		}

		masterIP, err := utils.GetMasterIP(request.NodeName)
		if err != nil {
			log.Printf("Failed to get master IP: %v\n", err)
			return c.Status(500).JSON(models.ProvisionResponse{
				Success: false,
				Error:   "Failed to get master IP: " + err.Error(),
			})
		}

		if err := utils.StoreProvisionInfo(request.NodeName, request.NodeUID, masterIP, request.NodeType, request.Token); err != nil {
			log.Printf("Failed to store provision info: %v\n", err)
			return c.Status(500).JSON(models.ProvisionResponse{
				Success: false,
				Error:   "Failed to store provision info: " + err.Error(),
			})
		}

		log.Printf("VM '%s' successfully provisioned with IP %s\n", request.NodeName, masterIP)
		return c.JSON(models.ProvisionResponse{
			Success:  true,
			Message:  fmt.Sprintf("VM '%s' successfully provisioned", request.NodeName),
			NodeID:   request.NodeUID,
			MasterIP: masterIP,
		})
	}
}

// DeleteVMHandler handles the deletion of a VM
func DeleteVMHandler(c *fiber.Ctx) error {
	vmName := c.Params("name")
	if vmName == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   "VM name is required",
		})
	}

	if err := utils.RunIgniteCommand("stop", vmName); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("Failed to stop VM: %v", err),
		})
	}

	if err := utils.RunIgniteCommand("rm", vmName); err != nil {
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

// Helper functions
func validateProvisionRequest(request *models.ProvisionRequest, nodeType string) error {
	if request.NodeName == "" || request.NodeUID == "" {
		return fmt.Errorf("NodeName and NodeUID are required fields")
	}
	if nodeType == "worker" && (request.MasterIP == "" || request.NodeType != "worker") {
		return fmt.Errorf("NodeName, NodeUID, MasterIP, and NodeType 'worker' are required fields")
	}
	if nodeType == "worker" && !utils.ValidateTokenAndMasterIP(request.Token, request.MasterIP) {
		return fmt.Errorf("Token and MasterIP do not match any existing records")
	}
	return nil
}

func createConfig(request *models.ProvisionRequest) config.Config {
	return config.Config{
		Name:     request.NodeName,
		UID:      request.NodeUID,
		NodeType: request.NodeType,
		Token:    request.Token,
		MasterIP: request.MasterIP,
	}
}

func createManifest(request *models.ProvisionRequest) config.Manifest {
	manifest := config.Manifest{
		APIVersion: "ignite.weave.works/v1alpha4",
		Kind:       "VM",
	}
	manifest.Metadata.Name = request.NodeName
	manifest.Metadata.UID = request.NodeUID

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

	log.Printf("Using image OCI: %s\n", imageOCI)

	manifest.Spec.Image = map[string]string{"oci": imageOCI}
	manifest.Spec.CPUs = cpus
	manifest.Spec.DiskSize = diskSize
	manifest.Spec.Memory = memory
	manifest.Spec.SSH = request.EnableSSH

	return manifest
}
