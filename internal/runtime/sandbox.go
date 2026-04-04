package runtime

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
	Available     bool   // Whether sandboxing is available on this platform
	Containerized bool   // Whether we're running inside a container
	ContainerType string // "docker", "podman", "kubernetes", or ""
	OS            string // Operating system
	Message       string // Human-readable status description
}

// DetectSandboxStatus probes the current environment and returns a SandboxStatus.
func DetectSandboxStatus() SandboxStatus {
	status := SandboxStatus{
		OS:        runtime.GOOS,
		Available: false,
	}

	// Check if we're inside a container.
	status.ContainerType = detectContainerType()
	status.Containerized = status.ContainerType != ""

	// Sandbox via unshare is only available on Linux.
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

// detectContainerType checks various indicators to determine if we're running
// inside a container and what kind.
func detectContainerType() string {
	// Check for Docker.
	if isDocker() {
		return "docker"
	}
	// Check for Podman.
	if isPodman() {
		return "podman"
	}
	// Check for Kubernetes.
	if isKubernetes() {
		return "kubernetes"
	}
	return ""
}

// isDocker checks for Docker-specific indicators.
func isDocker() bool {
	// Check for /.dockerenv file.
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// Check for Docker in /proc/1/cgroup.
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		if strings.Contains(string(data), "docker") {
			return true
		}
	}
	// Check for Docker-specific environment variables.
	if os.Getenv("DOCKER_CONTAINER") != "" || os.Getenv("DOCKER_ENV") != "" {
		return true
	}
	return false
}

// isPodman checks for Podman-specific indicators.
func isPodman() bool {
	// Check for Podman-specific environment variables.
	if os.Getenv("container") == "podman" {
		return true
	}
	// Check for /run/.containerenv (used by Podman).
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	return false
}

// isKubernetes checks for Kubernetes-specific indicators.
func isKubernetes() bool {
	// Check for Kubernetes service account token.
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount"); err == nil {
		return true
	}
	// Check for KUBERNETES_SERVICE_HOST environment variable.
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	return false
}

// SandboxCommandBuilder constructs sandboxed command invocations on Linux.
// On non-Linux platforms it returns the command unchanged.
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
// On non-Linux or when sandbox is unavailable, it returns the original name and args.
//
// The sandbox uses unshare to create new mount, PID, network, and user namespaces,
// effectively isolating the command from the host system.
func (b *SandboxCommandBuilder) WrapCommand(name string, args []string, workspaceRoot string) (string, []string) {
	if !b.status.Available {
		return name, args
	}

	// Check that unshare is available on the system.
	if _, err := exec.LookPath("unshare"); err != nil {
		return name, args
	}

	// Build the unshare wrapper command.
	// --mount: new mount namespace
	// --pid: new PID namespace
	// --net: new network namespace (no network access)
	// --map-root-user: map current user to root in the new namespace
	// --fork: fork before executing so PID namespace works correctly
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
// It returns warnings (not errors) for potentially unsafe operations.
func ValidateGitSafety(args ...string) []string {
	var warnings []string

	joined := strings.Join(args, " ")

	// Warn about --no-verify which skips git hooks.
	if strings.Contains(joined, "--no-verify") {
		warnings = append(warnings, "git --no-verify flag detected: this skips pre-commit hooks")
	}

	// Warn about --force which can rewrite history.
	if strings.Contains(joined, "--force") || strings.Contains(joined, "-f ") {
		// Allow force push to feature branches but warn.
		warnings = append(warnings, "git --force flag detected: this can rewrite history")
	}

	// Warn about --hard reset.
	if strings.Contains(joined, "--hard") {
		warnings = append(warnings, "git --hard flag detected: this discards uncommitted changes")
	}

	// Warn about clean -f which removes untracked files.
	if (strings.Contains(joined, "clean") && strings.Contains(joined, "-f")) ||
		strings.Contains(joined, "clean -f") {
		warnings = append(warnings, "git clean -f detected: this removes untracked files")
	}

	return warnings
}

// EnsureExplicitStaging validates that git staging is done explicitly (file by file)
// rather than blanket git add -A when in strict permission modes.
func EnsureExplicitStaging(mode PermissionMode, files []string) error {
	if mode == PermDangerFullAccess || mode == PermAllow {
		return nil // Full access modes allow blanket staging
	}

	if len(files) == 1 && (files[0] == "-A" || files[0] == ".") {
		if mode == PermReadOnly {
			return fmt.Errorf("read-only mode does not allow git staging")
		}
		// In workspace_write and prompt modes, blanket staging is allowed but
		// the caller should ideally use explicit file paths. We allow it but
		// don't error here.
	}
	return nil
}
