# Senechal Gateway: Configuration Design & Principles

**Version:** 1.1  
**Status:** Approved Specification  
**Focus:** Strict Monolithic Governance & Integrity

---

## 1. Core Philosophy: Single Root, Monolithic Governance

Senechal treats configuration as a **single, immutable tree** rooted at a primary entry point. Every decision made by the system is governed by this unified "World View."

### 1.1 The Single Root
- Every instance of Senechal Gateway starts with exactly **one** root file (e.g., `config.yaml`).
- This file is the "Source of Truth" from which all other configuration is discovered.

### 1.2 Strict Recursive Includes
- **Top-of-File Requirement**: The `include:` key MUST be an array and MUST appear at the very top of the YAML file. This allows for safe, restricted discovery of dependencies before full parsing.
- **Relative Pathing**: All include paths are resolved relative to the file that includes them.
- **Tree Jail**: Forbid paths that use `..` to go above the directory containing the root configuration file. Everything must live within the configuration root.
- **No Symlinks**: The system will refuse to load or hash any file that is a symbolic link. This prevents path traversal and "jailbreak" attacks.

### 1.3 Monolithic Grafting (Merge Logic)
- **n-1 Branching**: At runtime, the entire include tree is compiled into a single, monolithic configuration object.
- **Matching Branch Merge**: When an included file contains a top-level key that already exists in the monolith, the branches are merged.
    - **Maps (e.g., `plugins:`)**: Keys are merged. If a plugin name exists in both, the version from the *later* file overrides the *earlier* one.
    - **Arrays (e.g., `pipelines:`, `routes:`)**: Items are appended to the monolithic list.
    - **Scalars (e.g., `log_level:`)**: The later value replaces the earlier one.
- **Merge Order**: The root file defines the base. Includes are merged in the order they are listed.
- **Precedence**: Later entries override earlier ones. The "monolith" is the final, resolved state.

#### Example: Modular Pipelines
**config.yaml (Root)**
```yaml
include:
  - service_defaults.yaml
  - pipelines/ingestion.yaml
  - pipelines/alerts.yaml
service:
  name: prod-gateway
```

**pipelines/ingestion.yaml**
```yaml
pipelines:
  - name: video-wisdom
    on: discord.link
```

**pipelines/alerts.yaml**
```yaml
pipelines:
  - name: error-notify
    on: plugin.failure
```

**Resulting Monolith:**
```yaml
service:
  name: prod-gateway  # From root
  log_level: info     # From service_defaults.yaml
pipelines:            # Both files appended to matching branch
  - name: video-wisdom
  - name: error-notify
```

---

## 2. Integrity & Governance: The "Seal"

Security and reliability are enforced via a strict "Check-then-Load" sequence.

### 2.1 The Checksum Manifest (`.checksums`)
- The configuration tree is "sealed" using a monolithic manifest file located at the configuration root.
- **Signature = Absolute Path + Content**: The manifest contains hashes indexed by the **absolute path** of every file in the tree.
- **System Lock-in**: If you move the configuration directory, the seal is broken. The operator must intentionally re-sign the configuration in its new location.

### 2.2 Hashes Checked Before Use
- **Strict Verification**: The system MUST verify the BLAKE3 hash of every configuration file *before* its content is read, parsed, or merged.
- If a file is missing from the manifest, or its hash does not match, the system **must hard-fail** with a clear security warning.

### 2.3 The "Lock" Command
- The `config lock` command is the authoritative "compiler" and "signer."
- It performs a full discovery of the tree and generates a new `.checksums` file.
- It provides a safe "dry-run" capability to preview the tree before sealing.

---

## 3. Robustness & Safety

### 3.1 Loop Detection
- The loader MUST implement cycle detection to prevent infinite recursion.
- Circular dependencies are hard configuration errors.

### 3.2 Environment Interpolation
- Environment variable interpolation (`${VAR}`) happens *after* integrity verification but *before* parsing.
- Interpolation is forbidden in `include:` paths to ensure the tree structure remains static and verifiable.

---

## 4. The Operator Experience (LLM & Human)

- **Surgicality**: Modular source files allow an LLM to "edit the routes" without touching "service settings."
- **Fail-Fast with UX**: Any integrity or logic error results in an immediate exit with `78 (EX_CONFIG)`. Error messages must be actionable, providing the exact command needed to resolve the issue (e.g., "Run 'senechal-gw config lock'").
