package helpers

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Docker security validation patterns
var (
	// Allow only alphanumeric characters, hyphens, underscores, and dots for service names
	validServiceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	// Allow only safe characters for project names (alphanumeric, hyphens, underscores)
	validProjectNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	// Allow only safe file paths (no path traversal)
	validFilePathPattern = regexp.MustCompile(`^[a-zA-Z0-9_./:-]+$`)
)

// validateDockerInputs ensures Docker command inputs are safe
func validateDockerInputs(composeFile, projectName string, services []string) error {
	// Validate compose file path
	if !validFilePathPattern.MatchString(composeFile) || strings.Contains(composeFile, "..") {
		return fmt.Errorf("invalid compose file path: %s", composeFile)
	}

	// Validate project name
	if !validProjectNamePattern.MatchString(projectName) {
		return fmt.Errorf("invalid project name: %s", projectName)
	}

	// Validate service names
	for _, service := range services {
		if !validServiceNamePattern.MatchString(service) {
			return fmt.Errorf("invalid service name: %s", service)
		}
	}

	return nil
}

// validateContainerID ensures container ID is safe (hexadecimal characters only)
func validateContainerID(containerID string) bool {
	if len(containerID) < 12 || len(containerID) > 64 {
		return false
	}
	validContainerIDPattern := regexp.MustCompile(`^[a-fA-F0-9]+$`)
	return validContainerIDPattern.MatchString(containerID)
}

// DockerTestEnvironment manages Docker containers for integration testing
type DockerTestEnvironment struct {
	ComposeFile string
	ProjectName string
	Services    []string
}

// NewDockerTestEnvironment creates a new Docker test environment
func NewDockerTestEnvironment(t *testing.T) *DockerTestEnvironment {
	t.Helper()

	return &DockerTestEnvironment{
		ComposeFile: "../integration/docker-compose.yml",
		ProjectName: fmt.Sprintf("raven-test-%d", time.Now().Unix()),
		Services:    []string{"raven-imap", "raven-lmtp"},
	}
}

// Start starts the Docker test environment
func (d *DockerTestEnvironment) Start(t *testing.T) {
	t.Helper()

	// Validate inputs for security
	if err := validateDockerInputs(d.ComposeFile, d.ProjectName, d.Services); err != nil {
		t.Fatalf("Invalid Docker inputs: %v", err)
	}

	// Check if Docker is available
	if !d.isDockerAvailable(t) {
		t.Skip("Docker not available, skipping integration test")
	}

	// Start services
	composeFile, err := filepath.Abs(d.ComposeFile)
	if err != nil {
		t.Fatalf("Failed to resolve compose file path: %v", err)
	}
	cmd := exec.Command("docker-compose", // #nosec G204 - Validated inputs in test environment
		"-f", composeFile,
		"-p", d.ProjectName,
		"up", "-d",
	)
	cmd.Args = append(cmd.Args, d.Services...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start Docker environment: %v\nOutput: %s", err, output)
	}

	// Wait for services to be healthy
	d.waitForHealthy(t)
	t.Logf("Docker test environment started with project name: %s", d.ProjectName)
}

// Stop stops and cleans up the Docker test environment
func (d *DockerTestEnvironment) Stop(t *testing.T) {
	t.Helper()

	// Validate inputs for security
	if err := validateDockerInputs(d.ComposeFile, d.ProjectName, d.Services); err != nil {
		t.Logf("Warning: Invalid Docker inputs during cleanup: %v", err)
		return
	}

	// Stop and remove containers
	composeFile, err := filepath.Abs(d.ComposeFile)
	if err != nil {
		t.Logf("Warning: Failed to resolve compose file path during cleanup: %v", err)
		return
	}

	cmd := exec.Command("docker-compose", // #nosec G204 - Validated inputs in test environment
		"-f", composeFile,
		"-p", d.ProjectName,
		"down", "-v", "--remove-orphans",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: Failed to stop Docker environment: %v\nOutput: %s", err, output)
	} else {
		t.Logf("Docker test environment stopped and cleaned up")
	}
}

// GetServiceURL returns the URL for a service
func (d *DockerTestEnvironment) GetServiceURL(service string, port int) string {
	switch service {
	case "raven-imap":
		return fmt.Sprintf("127.0.0.1:%d", 10143)
	case "raven-lmtp":
		return fmt.Sprintf("127.0.0.1:%d", 10024)
	case "raven-full":
		switch port {
		case 143:
			return "127.0.0.1:10143"
		case 24:
			return "127.0.0.1:10024"
		}
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// isDockerAvailable checks if Docker and Docker Compose are available
func (d *DockerTestEnvironment) isDockerAvailable(t *testing.T) bool {
	t.Helper()

	// Check Docker
	cmd := exec.Command("docker", "version")
	if err := cmd.Run(); err != nil {
		t.Logf("Docker not available: %v", err)
		return false
	}

	// Check Docker Compose
	cmd = exec.Command("docker-compose", "version")
	if err := cmd.Run(); err != nil {
		t.Logf("Docker Compose not available: %v", err)
		return false
	}

	return true
}

// waitForHealthy waits for all services to become healthy
func (d *DockerTestEnvironment) waitForHealthy(t *testing.T) {
	t.Helper()

	timeout := 60 * time.Second
	interval := 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for Docker services to become healthy")
		case <-ticker.C:
			if d.checkServicesHealthy(t) {
				return
			}
		}
	}
}

// checkServicesHealthy checks if all services are healthy
func (d *DockerTestEnvironment) checkServicesHealthy(t *testing.T) bool {
	t.Helper()

	// Validate inputs for security
	if err := validateDockerInputs(d.ComposeFile, d.ProjectName, d.Services); err != nil {
		t.Logf("Invalid Docker inputs during health check: %v", err)
		return false
	}

	composeFile, err := filepath.Abs(d.ComposeFile)
	if err != nil {
		t.Logf("Failed to resolve compose file path during health check: %v", err)
		return false
	}

	cmd := exec.Command("docker-compose", // #nosec G204 - Validated inputs in test environment
		"-f", composeFile,
		"-p", d.ProjectName,
		"ps", "-q",
	)

	output, err := cmd.Output()
	if err != nil {
		t.Logf("Failed to check service status: %v", err)
		return false
	}

	containerIDs := strings.Fields(string(output))
	if len(containerIDs) == 0 {
		t.Logf("No containers found")
		return false
	}

	// Check each container with validated IDs
	for _, containerID := range containerIDs {
		// Validate container ID for security
		if !validateContainerID(containerID) {
			t.Logf("Invalid container ID format: %s", containerID)
			return false
		}

		cmd := exec.Command("docker", "inspect", // #nosec G204 - Validated container ID in test environment
			"--format={{.State.Health.Status}}", containerID)

		output, err := cmd.Output()
		if err != nil {
			t.Logf("Failed to inspect container %s: %v", containerID, err)
			return false
		}

		health := strings.TrimSpace(string(output))
		if health != "healthy" && health != "" {
			t.Logf("Container %s health status: %s", containerID, health)
			return false
		}
	}

	t.Logf("All Docker services are healthy")
	return true
}

// StartFullEnvironment starts the full integrated environment
func (d *DockerTestEnvironment) StartFullEnvironment(t *testing.T) {
	t.Helper()

	// Validate inputs for security
	if err := validateDockerInputs(d.ComposeFile, d.ProjectName, d.Services); err != nil {
		t.Fatalf("Invalid Docker inputs: %v", err)
	}

	// Check if Docker is available
	if !d.isDockerAvailable(t) {
		t.Skip("Docker not available, skipping e2e test")
	}

	// Start full service
	composeFile, err := filepath.Abs(d.ComposeFile)
	if err != nil {
		t.Fatalf("Failed to resolve compose file path: %v", err)
	}

	cmd := exec.Command("docker-compose", // #nosec G204 - Validated inputs in test environment
		"-f", composeFile,
		"-p", d.ProjectName,
		"up", "-d", "raven-full",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start full Docker environment: %v\nOutput: %s", err, output)
	}

	// Wait for services to be healthy
	d.waitForHealthy(t)
	t.Logf("Full Docker environment started with project name: %s", d.ProjectName)
}

// StartSeparateServices starts individual services separately
func (d *DockerTestEnvironment) StartSeparateServices(t *testing.T) {
	t.Helper()

	// Validate inputs for security
	if err := validateDockerInputs(d.ComposeFile, d.ProjectName, d.Services); err != nil {
		t.Fatalf("Invalid Docker inputs: %v", err)
	}

	// Check if Docker is available
	if !d.isDockerAvailable(t) {
		t.Skip("Docker not available, skipping integration test")
	}

	// Start separate services
	composeFile, err := filepath.Abs(d.ComposeFile)
	if err != nil {
		t.Fatalf("Failed to resolve compose file path: %v", err)
	}

	cmd := exec.Command("docker-compose", // #nosec G204 - Validated inputs in test environment
		"-f", composeFile,
		"-p", d.ProjectName,
		"up", "-d",
	)
	cmd.Args = append(cmd.Args, d.Services...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start separate Docker services: %v\nOutput: %s", err, output)
	}

	// Wait for services to be healthy
	d.waitForHealthy(t)
	t.Logf("Separate Docker services started with project name: %s", d.ProjectName)
}

// Logs gets the logs from a specific service
func (d *DockerTestEnvironment) Logs(t *testing.T, service string) string {
	t.Helper()

	// Validate service name for security
	if !validServiceNamePattern.MatchString(service) {
		t.Logf("Invalid service name: %s", service)
		return ""
	}

	// Validate inputs for security
	if err := validateDockerInputs(d.ComposeFile, d.ProjectName, d.Services); err != nil {
		t.Logf("Invalid Docker inputs during log retrieval: %v", err)
		return ""
	}

	composeFile, err := filepath.Abs(d.ComposeFile)
	if err != nil {
		t.Logf("Failed to resolve compose file path during log retrieval: %v", err)
		return ""
	}

	cmd := exec.Command("docker-compose", // #nosec G204 - Validated inputs in test environment
		"-f", composeFile,
		"-p", d.ProjectName,
		"logs", service,
	)

	output, err := cmd.Output()
	if err != nil {
		t.Logf("Failed to get logs for %s: %v", service, err)
		return ""
	}

	return string(output)
}
