package helpers

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

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
	// Check if Docker is available
	if !d.isDockerAvailable(t) {
		t.Skip("Docker not available, skipping integration test")
	}

	// Start services
	cmd := exec.Command("docker-compose",
		"-f", d.ComposeFile,
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

	// Stop and remove containers
	cmd := exec.Command("docker-compose",
		"-f", d.ComposeFile,
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

	cmd := exec.Command("docker-compose",
		"-f", d.ComposeFile,
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

	// Check each container
	for _, containerID := range containerIDs {
		cmd := exec.Command("docker", "inspect",
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

	// Check if Docker is available
	if !d.isDockerAvailable(t) {
		t.Skip("Docker not available, skipping e2e test")
	}

	// Start full service
	cmd := exec.Command("docker-compose",
		"-f", d.ComposeFile,
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

	// Check if Docker is available
	if !d.isDockerAvailable(t) {
		t.Skip("Docker not available, skipping integration test")
	}

	// Start separate services
	cmd := exec.Command("docker-compose",
		"-f", d.ComposeFile,
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

	cmd := exec.Command("docker-compose",
		"-f", d.ComposeFile,
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
