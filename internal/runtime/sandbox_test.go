package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSandboxStatus(t *testing.T) {
	status := DetectSandboxStatus()
	if status.OS == "" {
		t.Error("OS should not be empty")
	}
	// On macOS (our test environment), sandbox should be unavailable.
	if status.OS == "darwin" && status.Available {
		t.Error("sandbox should not be available on darwin")
	}
}

func TestIsDocker(t *testing.T) {
	// We can't easily test positive detection in unit tests, but we can verify
	// it doesn't panic and returns false when not in Docker.
	result := isDocker()
	// On macOS test runner, this should be false.
	_ = result
}

func TestIsPodman(t *testing.T) {
	result := isPodman()
	_ = result
}

func TestIsKubernetes(t *testing.T) {
	result := isKubernetes()
	_ = result
}

func TestSandboxCommandBuilderNonLinux(t *testing.T) {
	builder := NewSandboxCommandBuilder()

	name := "bash"
	args := []string{"-c", "echo hello"}

	wrappedName, wrappedArgs := builder.WrapCommand(name, args, "/tmp")

	if builder.status.OS != "linux" {
		// On non-Linux, command should be returned unchanged.
		if wrappedName != name {
			t.Errorf("WrapCommand name = %q, want %q on non-Linux", wrappedName, name)
		}
		if len(wrappedArgs) != len(args) {
			t.Errorf("WrapCommand args = %v, want %v on non-Linux", wrappedArgs, args)
		}
	}
}

func TestSecurityChecks(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name         string
		workspace    string
		expectErrors int
	}{
		{"valid workspace", tmpDir, 0},
		{"nonexistent workspace", "/nonexistent/path/xyz", 1},
		{"relative path", "relative/path", 2}, // not absolute and doesn't exist
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := RunSecurityChecks(tt.workspace)
			if len(errs) != tt.expectErrors {
				t.Errorf("RunSecurityChecks(%q) returned %d errors, want %d",
					tt.workspace, len(errs), tt.expectErrors)
			}
		})
	}
}

func TestSecurityChecksDotDot(t *testing.T) {
	errs := RunSecurityChecks("/tmp/../etc")
	// This may or may not have errors depending on path cleaning, but shouldn't panic.
	_ = errs
}

func TestValidateGitSafety(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		warnings int
	}{
		{"clean commit", []string{"commit", "-m", "message"}, 0},
		{"no-verify", []string{"commit", "--no-verify", "-m", "msg"}, 1},
		{"force push", []string{"push", "--force"}, 1},
		{"hard reset", []string{"reset", "--hard", "HEAD~1"}, 1},
		{"clean -f", []string{"clean", "-f"}, 1},
		{"multiple flags", []string{"commit", "--no-verify", "--force"}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := ValidateGitSafety(tt.args...)
			if len(warnings) != tt.warnings {
				t.Errorf("ValidateGitSafety(%v) = %d warnings, want %d",
					tt.args, len(warnings), tt.warnings)
			}
		})
	}
}

func TestEnsureExplicitStaging(t *testing.T) {
	tests := []struct {
		mode    PermissionMode
		files   []string
		wantErr bool
	}{
		{PermDangerFullAccess, []string{"-A"}, false},
		{PermAllow, []string{"."}, false},
		{PermReadOnly, []string{"-A"}, true},
		{PermWorkspaceWrite, []string{"file.txt"}, false},
		{PermWorkspaceWrite, []string{"-A"}, false}, // blanket allowed but not ideal
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			err := EnsureExplicitStaging(tt.mode, tt.files)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureExplicitStaging(%q, %v) = %v, wantErr %v",
					tt.mode, tt.files, err, tt.wantErr)
			}
		})
	}
}

func TestSecurityCheckWorkspaceNotDir(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	errs := RunSecurityChecks(tmpFile)
	if len(errs) == 0 {
		t.Error("expected error when workspace root is a file, not a directory")
	}
}
