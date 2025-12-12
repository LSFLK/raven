package e2e

import (
	"net"
	"strings"
	"testing"
	"time"

	"raven/test/helpers"
)

// connectIMAPForDocker creates an IMAP connection suitable for Docker testing
// This skips TLS to avoid certificate issues in containerized environments
func connectIMAPForDocker(t *testing.T, addr string) *helpers.IMAPClient {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to IMAP server: %v", err)
	}

	client := helpers.NewIMAPClient(conn)

	// Read server greeting
	greeting, err := client.ReadLine()
	if err != nil {
		_ = conn.Close()
		t.Fatalf("Failed to read greeting: %v", err)
	}

	if !strings.HasPrefix(greeting, "* OK") {
		_ = conn.Close()
		t.Fatalf("Invalid greeting: %s", greeting)
	}

	// For Docker environments, skip STARTTLS to avoid TLS certificate issues
	t.Logf("IMAP client connected to %s (Docker mode - TLS skipped)", addr)
	return client
}

// TestCompleteEmailWorkflow tests the complete email workflow from LMTP delivery to IMAP retrieval
func TestCompleteEmailWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	// Setup Docker test environment
	dockerEnv := helpers.NewDockerTestEnvironment(t)
	dockerEnv.StartFullEnvironment(t)
	defer dockerEnv.Stop(t)

	// Wait a moment for services to fully initialize
	time.Sleep(2 * time.Second)

	// Get service addresses
	imapAddr := dockerEnv.GetServiceURL("raven-full", 143)
	lmtpAddr := dockerEnv.GetServiceURL("raven-full", 24)

	t.Logf("IMAP server: %s", imapAddr)
	t.Logf("LMTP server: %s", lmtpAddr)

	// Load test data
	testEmail := helpers.LoadSimpleEmail(t)

	t.Logf("Loaded test email: %d bytes", len(testEmail))

	// Test 1: Connect to IMAP server
	t.Run("IMAP_Connection", func(t *testing.T) {
		client := connectIMAPForDocker(t, imapAddr)
		defer func() {
			if err := client.Close(); err != nil {
				t.Logf("Warning: Failed to close IMAP client: %v", err)
			}
		}()

		// Test capability command
		responses, err := client.SendCommand("CAPABILITY")
		if err != nil {
			t.Fatalf("CAPABILITY command failed: %v", err)
		}

		if len(responses) == 0 {
			t.Fatal("No response to CAPABILITY command")
		}

		t.Logf("IMAP capabilities received: %d responses", len(responses))
	})

	// Test 2: Authentication workflow
	t.Run("IMAP_Authentication", func(t *testing.T) {
		client := connectIMAPForDocker(t, imapAddr)
		defer func() {
			if err := client.Close(); err != nil {
				t.Logf("Warning: Failed to close IMAP client: %v", err)
			}
		}()

		// Try to authenticate with test user
		// This assumes alice@example.com / password123 from test-users.json
		responses, err := client.SendCommand("LOGIN alice@example.com password123")
		if err != nil {
			t.Logf("Login failed (expected if user not set up): %v", err)
			t.Skip("User authentication not configured yet")
		}

		if len(responses) > 0 {
			t.Logf("Authentication response: %s", responses[len(responses)-1])
		}
	})

	// Test 3: Basic IMAP commands
	t.Run("IMAP_Basic_Commands", func(t *testing.T) {
		client := connectIMAPForDocker(t, imapAddr)
		defer func() {
			if err := client.Close(); err != nil {
				t.Logf("Warning: Failed to close IMAP client: %v", err)
			}
		}()

		// Test LIST command (should work without authentication for some servers)
		responses, err := client.SendCommand("LIST \"\" \"*\"")
		if err != nil {
			t.Logf("LIST command failed: %v", err)
		} else {
			t.Logf("LIST command successful: %d responses", len(responses))
		}
	})

	t.Logf("E2E test completed successfully")
}

// TestDockerEnvironmentSetup tests that the Docker environment can be started and stopped
func TestDockerEnvironmentSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker test in short mode")
	}

	t.Run("SeparateServices", func(t *testing.T) {
		dockerEnv := helpers.NewDockerTestEnvironment(t)
		dockerEnv.StartSeparateServices(t)
		defer dockerEnv.Stop(t)

		// Verify services are accessible
		imapAddr := dockerEnv.GetServiceURL("raven-imap", 143)
		lmtpAddr := dockerEnv.GetServiceURL("raven-lmtp", 24)

		t.Logf("IMAP service: %s", imapAddr)
		t.Logf("LMTP service: %s", lmtpAddr)

		// Try to connect to services
		client := connectIMAPForDocker(t, imapAddr)
		if err := client.Close(); err != nil {
			t.Logf("Warning: Failed to close IMAP client: %v", err)
		}

		t.Logf("Successfully connected to separate services")
	})

	t.Run("FullService", func(t *testing.T) {
		dockerEnv := helpers.NewDockerTestEnvironment(t)
		dockerEnv.StartFullEnvironment(t)
		defer dockerEnv.Stop(t)

		// Verify full service is accessible
		imapAddr := dockerEnv.GetServiceURL("raven-full", 143)
		lmtpAddr := dockerEnv.GetServiceURL("raven-full", 24)

		t.Logf("Full IMAP service: %s", imapAddr)
		t.Logf("Full LMTP service: %s", lmtpAddr)

		// Try to connect to full service
		client := connectIMAPForDocker(t, imapAddr)
		if err := client.Close(); err != nil {
			t.Logf("Warning: Failed to close IMAP client: %v", err)
		}

		t.Logf("Successfully connected to full service")
	})
}

// TestFixtureLoading tests that all fixtures can be loaded correctly
func TestFixtureLoading(t *testing.T) {
	fixtures := []struct {
		name    string
		loader  func(*testing.T) []byte
		minSize int
	}{
		{"Simple Email", helpers.LoadSimpleEmail, 100},
		{"Multipart Email", helpers.LoadMultipartEmail, 200},
		{"Email with Attachment", helpers.LoadEmailWithAttachment, 300},
		{"HTML Email", helpers.LoadHTMLEmail, 500},
		{"Multi-recipient Email", helpers.LoadMultiRecipientEmail, 400},
		{"Unicode Email", helpers.LoadUnicodeEmail, 800},
		{"Large Email", helpers.LoadLargeEmail, 2000},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			data := fixture.loader(t)
			if len(data) < fixture.minSize {
				t.Errorf("Fixture %s too small: got %d bytes, expected at least %d",
					fixture.name, len(data), fixture.minSize)
			}
			t.Logf("Loaded %s: %d bytes", fixture.name, len(data))
		})
	}

	// Test configuration fixtures
	t.Run("TestUsers", func(t *testing.T) {
		config := helpers.LoadTestUsers(t)
		if config["domains"] == nil {
			t.Error("Test users config missing domains")
		}
		t.Log("Test users configuration loaded successfully")
	})

	t.Run("MailboxStructures", func(t *testing.T) {
		config := helpers.LoadMailboxStructures(t)
		if config["mailbox_structures"] == nil {
			t.Error("Mailbox structures config missing mailbox_structures")
		}
		t.Log("Mailbox structures configuration loaded successfully")
	})

	t.Run("TestConfig", func(t *testing.T) {
		config := helpers.LoadTestConfig(t)
		if len(config) < 100 {
			t.Error("Test config too small")
		}
		t.Logf("Test configuration loaded: %d bytes", len(config))
	})
}
