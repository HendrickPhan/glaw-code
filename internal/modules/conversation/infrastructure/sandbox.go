package infrastructure

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SandboxStatus describes the current sandbox environment.
type SandboxStatus struct {
	Available     bool
	Containerized bool
	ContainerType string
	OS            string
	Message       string
}

// DetectSandboxStatus probes the current environment and returns a SandboxStatus.
func DetectSandboxStatus() SandboxStatus {
	status := SandboxStatus{
		OS:        runtime.GOOS,
		Available: false,
	}

	status.ContainerType = detectContainerType()
	status.Containerized = status.ContainerType != ""

	if runtime.GOOS == "linux" {
		status.Available = true
		if status.Containerized {
			status.Message = fmt.Sprintf("Running inside %s container; sandbox via unshare available", status.ContainerType)
		} else {
			status.Message = "Linux detected; sandbox via unshare available"
		}
	} else {
		status.Message = fmt.Sprintf("Sandbox unavailable on %s (requires Linux with unshare)", runtime.GOOS)
	}

	return status
}

func detectContainerType() string {
	if isDocker() {
		return "docker"
	}
	if isPodman() {
		return "podman"
	}
	if isKubernetes() {
		return "kubernetes"
	}
	return ""
}

func isDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		if strings.Contains(string(data), "docker") {
			return true
		}
	}
	if os.Getenv("DOCKER_CONTAINER") != "" || os.Getenv("DOCKER_ENV") != "" {
		return true
	}
	return false
}

func isPodman() bool {
	if os.Getenv("container") == "podman" {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	return false
}

func isKubernetes() bool {
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount"); err == nil {
		return true
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	return false
}

// SandboxCommandBuilder constructs sandboxed command invocations on Linux.
type SandboxCommandBuilder struct {
	status SandboxStatus
}

// NewSandboxCommandBuilder creates a builder based on the current sandbox status.
func NewSandboxCommandBuilder() *SandboxCommandBuilder {
	return &SandboxCommandBuilder{
		status: DetectSandboxStatus(),
	}
}

// WrapCommand wraps a command with Linux sandboxing via unshare when available.
func (b *SandboxCommandBuilder) WrapCommand(name string, args []string, workspaceRoot string) (string, []string) {
	if !b.status.Available {
		return name, args
	}

	if _, err := exec.LookPath("unshare"); err != nil {
		return name, args
	}

	wrappedArgs := []string{
		"--mount", "--pid", "--net", "--map-root-user", "--fork",
		"--root", workspaceRoot,
		"--wd", workspaceRoot,
		name,
	}
	wrappedArgs = append(wrappedArgs, args...)

	return "unshare", wrappedArgs
}

// SecurityInvariant represents a security rule that should be enforced.
type SecurityInvariant struct {
	ID          string
	Description string
	Check       func() error
}

// SecurityChecks returns the list of security invariants to verify.
func SecurityChecks(workspaceRoot string) []SecurityInvariant {
	return []SecurityInvariant{
		{
			ID:          "workspace_exists",
			Description: "Workspace root must exist",
			Check: func() error {
				info, err := os.Stat(workspaceRoot)
				if err != nil {
					return fmt.Errorf("workspace root %q does not exist: %w", workspaceRoot, err)
				}
				if !info.IsDir() {
					return fmt.Errorf("workspace root %q is not a directory", workspaceRoot)
				}
				return nil
			},
		},
		{
			ID:          "workspace_absolute",
			Description: "Workspace root must be an absolute path",
			Check: func() error {
				if !filepath.IsAbs(workspaceRoot) {
					return fmt.Errorf("workspace root %q is not absolute", workspaceRoot)
				}
				return nil
			},
		},
		{
			ID:          "no_dotdot_in_path",
			Description: "Workspace root must not contain path traversal",
			Check: func() error {
				cleaned := filepath.Clean(workspaceRoot)
				if strings.Contains(cleaned, "..") {
					return fmt.Errorf("workspace root %q contains path traversal", workspaceRoot)
				}
				return nil
			},
		},
	}
}

// RunSecurityChecks executes all security invariant checks and returns any errors.
func RunSecurityChecks(workspaceRoot string) []error {
	var errs []error
	for _, check := range SecurityChecks(workspaceRoot) {
		if err := check.Check(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", check.ID, err))
		}
	}
	return errs
}

// ValidateGitSafety checks that git operations follow safety conventions.
func ValidateGitSafety(args ...string) []string {
	var warnings []string
	joined := strings.Join(args, " ")

	if strings.Contains(joined, "--no-verify") {
		warnings = append(warnings, "git --no-verify flag detected: this skips pre-commit hooks")
	}
	if strings.Contains(joined, "--force") || strings.Contains(joined, "-f ") {
		warnings = append(warnings, "git --force flag detected: this can rewrite history")
	}
	if strings.Contains(joined, "--hard") {
		warnings = append(warnings, "git --hard flag detected: this discards uncommitted changes")
	}
	if (strings.Contains(joined, "clean") && strings.Contains(joined, "-f")) ||
		strings.Contains(joined, "clean -f") {
		warnings = append(warnings, "git clean -f detected: this removes untracked files")
	}

	return warnings
}

// EnsureExplicitStaging validates that git staging is done explicitly.
func EnsureExplicitStaging(mode string, files []string) error {
	if mode == "danger_full_access" || mode == "allow" {
		return nil
	}

	if len(files) == 1 && (files[0] == "-A" || files[0] == ".") {
		if mode == "read_only" {
			return fmt.Errorf("read-only mode does not allow git staging")
		}
	}
	return nil
}

// OSName returns the current operating system name.
func OSName() string {
	return runtime.GOOS
}
