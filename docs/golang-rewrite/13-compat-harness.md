# Compat Harness — Upstream Manifest Extraction

## Package: `internal/compat/`

---

## Overview

The compat-harness extracts tool and command manifests from the upstream TypeScript source code. This is used during development to track parity between the Go port and the original TypeScript implementation.

---

## UpstreamPaths

```go
type UpstreamPaths struct {
    RepoRoot string
}
```

### Constructors

```go
func FromRepoRoot(root string) *UpstreamPaths
func FromWorkspaceDir(dir string) *UpstreamPaths
```

**FromWorkspaceDir resolution:**
```
1. Walk up from dir looking for:
   a. "claw-code" directory in ancestors
   b. "reference-source" directory in ancestors
   c. "vendor" subdirectory containing upstream source
2. Check CLAW_CODE_UPSTREAM env var
3. Return UpstreamPaths with resolved RepoRoot
```

### Path Accessors

```go
func (p *UpstreamPaths) CommandsPath() string  // {repo_root}/src/commands.ts
func (p *UpstreamPaths) ToolsPath() string      // {repo_root}/src/tools.ts
func (p *UpstreamPaths) CLIPath() string        // {repo_root}/src/entrypoints/cli.tsx
```

---

## ExtractedManifest

```go
type ExtractedManifest struct {
    Commands  CommandRegistry
    Tools     ToolRegistry
    Bootstrap BootstrapPlan
}
```

---

## Extraction Functions

### extract_commands(path) → CommandRegistry

**Algorithm:**
```
1. Read entire file content
2. Find all import statements:
   Pattern: "import { Symbol } from ..."
   Extract symbol names
3. Find INTERNAL_ONLY_COMMANDS block:
   Pattern: Look for assignment to variable containing "COMMANDS"
   Extract identifiers from the assignment
4. FOR EACH symbol:
   Classify source:
   - IF from INTERNAL_ONLY_COMMANDS: CommandSource = InternalOnly
   - IF name contains "FeatureGate" or similar: CommandSource = FeatureGated
   - ELSE: CommandSource = Builtin
5. Deduplicate commands (preserve order, remove dup names)
6. Return CommandRegistry with all extracted commands
```

### extract_tools(path) → ToolRegistry

**Algorithm:**
```
1. Read entire file content
2. Find all import statements importing *Tool symbols:
   Pattern: "import { SomethingTool } from ..."
   Extract symbol names ending in "Tool"
3. FOR EACH symbol:
   Classify source:
   - IF imported directly: ToolSource = Base
   - IF conditionally imported or guarded: ToolSource = Conditional
4. Deduplicate tools
5. Return ToolRegistry
```

### extract_bootstrap_plan(path) → BootstrapPlan

**Algorithm:**
```
1. Read entire file content (cli.tsx)
2. Search for string literals representing bootstrap phases:
   Known phase strings:
   - "CliEntry"
   - "FastPathVersion"
   - "StartupProfiler"
   - "SystemPromptFastPath"
   - "ChromeMcpFastPath"
   - etc.
3. Collect found phases in order of appearance
4. Return BootstrapPlan{Phases: found_phases}
```

---

## Helper Functions

### imported_symbols(lines) → []string

```
1. FOR each line in file:
   IF line matches "import ... from":
     Extract symbols between { and }
     Split by comma, trim whitespace
     Add to result
2. Return all extracted symbol names
```

### first_assignment_identifier(line) → string

```
1. Find first "=" in line
2. Take text before "="
3. Extract last identifier (word) from that text
4. Return identifier
```

### first_identifier(line) → string

```
1. Find first sequence of [a-zA-Z0-9_] characters
2. Return that sequence
```

### dedupe_commands/ dedupe_tools

```
1. Track seen set of names
2. Filter to first occurrence of each name
3. Return deduplicated list
```

---

## Integration Usage

This module is typically used in:
1. Development-time parity checks
2. CI verification that the Go port covers all upstream features
3. Generating documentation about the command/tool surface

It is NOT needed at runtime in production.
