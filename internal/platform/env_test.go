package goplatform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPort(t *testing.T) {
	// 1. Service var priority
	os.Setenv("TEST_SERVICE_PORT", "9001")
	os.Setenv("PORT", "4000")
	if p := Port("TEST_SERVICE_PORT", "PORT", "8000"); p != "9001" {
		t.Errorf("Expected 9001 from service var, got %s", p)
	}

	// 2. Matching _URL var extraction
	os.Unsetenv("TEST_SERVICE_PORT")
	os.Unsetenv("PORT")
	os.Setenv("TEST_SERVICE_URL", "http://localhost:9002/api")
	if p := Port("TEST_SERVICE_PORT", "PORT", "8000"); p != "9002" {
		t.Errorf("Expected 9002 from TEST_SERVICE_URL, got %s", p)
	}

	// 2b. NEXT_PUBLIC_ matching _URL var extraction
	os.Unsetenv("TEST_SERVICE_URL")
	os.Setenv("NEXT_PUBLIC_TEST_SERVICE_URL", "http://localhost:9003")
	if p := Port("TEST_SERVICE_PORT", "PORT", "8000"); p != "9003" {
		t.Errorf("Expected 9003 from NEXT_PUBLIC_TEST_SERVICE_URL, got %s", p)
	}
	os.Unsetenv("NEXT_PUBLIC_TEST_SERVICE_URL")

	// 3. Platform var fallback
	os.Unsetenv("TEST_SERVICE_URL")
	os.Setenv("PORT", "4000")
	if p := Port("TEST_SERVICE_PORT", "PORT", "8000"); p != "4000" {
		t.Errorf("Expected 4000 from platform var, got %s", p)
	}

	// 4. Hardcoded fallback
	os.Unsetenv("PORT")
	if p := Port("TEST_SERVICE_PORT", "PORT", "8000"); p != "8000" {
		t.Errorf("Expected 8000 from fallback, got %s", p)
	}
}

func TestExtractPort(t *testing.T) {
	cases := []struct {
		url      string
		expected string
	}{
		{"http://localhost:8081", "8081"},
		{"http://127.0.0.1:8082/api", "8082"},
		{"https://example.com:9000", "9000"},
		{"http://example.com", ""},
		{"localhost:8086", "8086"},
		{"", ""},
	}
	for _, c := range cases {
		if p := ExtractPort(c.url); p != c.expected {
			t.Errorf("ExtractPort(%q) = %q, expected %q", c.url, p, c.expected)
		}
	}
}

func TestFirst(t *testing.T) {
	os.Setenv("TEST_VAR_A", "")
	os.Setenv("TEST_VAR_B", "value_b")
	os.Setenv("TEST_VAR_C", "value_c")

	if v := First("TEST_VAR_A", "TEST_VAR_B", "TEST_VAR_C"); v != "value_b" {
		t.Errorf("Expected value_b, got %s", v)
	}

	if v := First("NON_EXISTENT_1", "NON_EXISTENT_2"); v != "" {
		t.Errorf("Expected empty string, got %s", v)
	}
}

func TestLoadDotEnvDiscovery(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "apps", "gateway", "cmd")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dirs: %v", err)
	}

	envContent := "TEST_DISCOVERY_KEY=discovered_successfully\n"
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("Failed to write .env: %v", err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	os.Unsetenv("TEST_DISCOVERY_KEY")
	LoadDotEnv()

	if val := os.Getenv("TEST_DISCOVERY_KEY"); val != "discovered_successfully" {
		t.Errorf("Expected discovered_successfully, got '%s'", val)
	}
}
