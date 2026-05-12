# Investigation: VPN-Dependent Pre-commit Hook in hyperfleet-api

## Executive Summary

The hyperfleet-api repository includes `rh-pre-commit` in its `.pre-commit-config.yaml`, which requires VPN access to Red Hat's internal GitLab during `pre-commit install`. This blocks developers who are offline or on unstable VPN connections.

**Root Cause**: The hook repository is hosted on `gitlab.cee.redhat.com` (Red Hat internal GitLab).

**Solution**: Use `rh-multi-pre-commit` global installation instead of including `rh-pre-commit` in the project's `.pre-commit-config.yaml`.

---

## The Problem

**Current configuration** (`.pre-commit-config.yaml`):
```yaml
repos:
  - repo: https://gitlab.cee.redhat.com/infosec-public/developer-workbench/tools
    rev: rh-pre-commit-2.3.2
    hooks:
      - id: rh-pre-commit
```

**Impact**:
- ❌ External contributors cannot install hooks (no VPN access)
- ❌ Red Hat associates cannot clone and setup without VPN
- ❌ Forces VPN dependency for every new clone
- ⚠️ Developers bypass all hooks with `--no-verify`

---

## The Solution: rh-multi-pre-commit

### What is rh-multi-pre-commit?

**Quick distinction**:
- **`rh-pre-commit`** — The security hook that scans for secrets (using rh-gitleaks)
- **`rh-multi-pre-commit`** — Hook manager that runs rh-pre-commit globally + project hooks simultaneously

From [InfoSec documentation](https://gitlab.cee.redhat.com/infosec-public/developer-workbench/tools/-/tree/main/rh-pre-commit):

> "As rh-pre-commit is currently internal tooling it will only be able to install when you are connected to the VPN. For external repositories this could be problematic, so it is recommended that **rh-multi-pre-commit is used instead**. This allows you to still run existing pre-commit hooks, as desired, whilst keeping the configuration local."

**Key Features**:
- **Global installation** — runs on all repositories automatically
- **VPN required only once** — during initial installation
- **Works offline** — after installation, no VPN needed for commits
- **Local configuration** — settings stored in `~/.config/rh-multi-pre-commit/`
- **Extends pre-commit.com** — adds global config support to pre-commit framework

### How It Works

**One-time setup (requires VPN)**:

> **Security Note**: The command below pipes a remote script directly to bash. Only execute this on a trusted network (Red Hat VPN) from a trusted source. For additional security, you can download and inspect the script before executing: `curl -o /tmp/quickstart.sh https://gitlab.cee.redhat.com/infosec-public/developer-workbench/tools/-/raw/main/rh-pre-commit/quickstart.sh && less /tmp/quickstart.sh && bash /tmp/quickstart.sh -f`

```bash
# Connect to Red Hat VPN first
# Then run (installs globally for all repos):
curl -s https://gitlab.cee.redhat.com/infosec-public/developer-workbench/tools/-/raw/main/rh-pre-commit/quickstart.sh | bash -s -- -f

# OR install for specific directory only:
# curl -s https://gitlab.cee.redhat.com/...quickstart.sh | bash -s -- -r ~/projects
```

**Arguments**:
- `-f` — Install globally in all git repos under `$HOME` (recommended)
- `-r [directory]` — Install only in specific repos directory
- `-s` — Include sign-off hook (DCO)
- `-b [branch]` — Install on specific branch only

This installs `rh-multi-pre-commit` globally. After installation:

**Daily workflow (no VPN required)**:
```bash
git clone https://github.com/openshift-hyperfleet/hyperfleet-api
pre-commit install  # Installs project hooks only
git commit -m "feat: add feature"
# Runs: project hooks + rh-pre-commit (global) — automatically!
```

### Why This Approach?

From [rh-pre-commit FAQ](https://source.redhat.com/departments/strategy_and_operations/it/it_information_security/leaktk/leaktk_components/rh_pre_commit):

**Q: "Does the hook require the VPN to work?"**  
**A: "No, it only requires the VPN to do an install but after that, everything works outside the VPN."**

**Benefits**:
- ✅ External contributors unaffected (don't install rh-multi-pre-commit)
- ✅ Red Hat associates get automatic security scanning
- ✅ No VPN required for daily development
- ✅ No need to bypass hooks with `--no-verify`
- ✅ Configuration is global, not per-project

---

## Recommended Changes

### For hyperfleet-api Repository

**Option 1: Remove rh-pre-commit from `.pre-commit-config.yaml`** (Recommended by InfoSec)

Keep only public hooks:
```yaml
repos:
  - repo: https://github.com/openshift-hyperfleet/rh-hooks-ai
    rev: v1.0.4
    hooks:
      - id: check-rh-precommit
      - id: validate-agents-md
      - id: ai-attribution-reminder
```

**Option 2: Keep current config but document limitation**

Add clear documentation that external contributors and Red Hat associates without VPN should skip this hook:
```bash
SKIP=rh-pre-commit git commit -m "your message"
```

**Recommended: Option 1** — aligns with InfoSec guidance for external repositories.

### For External Contributors (No Red Hat VPN)

**Setup** (no VPN required):
```bash
# Clone repository
git clone https://github.com/openshift-hyperfleet/hyperfleet-api

# Install pre-commit framework
pip install pre-commit

# Install project hooks
pre-commit install
pre-commit install --hook-type pre-push
```

**Daily usage**:
```bash
git commit -m "feat: your message"
# Runs only public hooks from .pre-commit-config.yaml
```

**Note**: External contributors do **NOT** install `rh-multi-pre-commit` — it's Red Hat internal tooling and requires VPN access. They only run the public hooks defined in the project's `.pre-commit-config.yaml`.

---

### For Red Hat Associates (With VPN Access)

**One-time installation** (requires VPN):

> **Security Note**: The command below pipes a remote script directly to bash. Only execute this on a trusted network (Red Hat VPN) from a trusted source. For additional security, you can download and inspect the script before executing: `curl -o /tmp/quickstart.sh https://gitlab.cee.redhat.com/infosec-public/developer-workbench/tools/-/raw/main/rh-pre-commit/quickstart.sh && less /tmp/quickstart.sh && bash /tmp/quickstart.sh -f`

```bash
# Connect to Red Hat VPN
# Run quickstart script (installs globally for all repos):
curl -s https://gitlab.cee.redhat.com/infosec-public/developer-workbench/tools/-/raw/main/rh-pre-commit/quickstart.sh | bash -s -- -f

# OR install for specific directory only:
# curl -s https://gitlab.cee.redhat.com/...quickstart.sh | bash -s -- -r ~/projects

# Verify installation:
which rh-multi-pre-commit
```

**What `-f` does**: Installs rh-multi-pre-commit globally in all git repositories under `$HOME` and overwrites existing pre-commit hooks (applies the `--force` flag internally).

**Alternative options**:
- `-r [directory]` — Install only in specific repos directory
- `-s` — Include sign-off hook (DCO)
- `-b [branch]` — Install on specific branch only

**Daily usage** (VPN optional):
```bash
git clone https://github.com/openshift-hyperfleet/hyperfleet-api
pre-commit install  # Install project hooks
git commit -m "feat: your message"
# Runs: project hooks + rh-pre-commit (global) — automatically!
```

**Pattern updates** (automatic):
- Patterns are automatically updated every 12 hours when committing **if VPN is connected**
- If offline: uses cached patterns (may show warning if cache is stale)
- Manual update only needed if InfoSec announces critical pattern release:
  ```bash
  # Connect to VPN, then:
  rh-multi-pre-commit update
  ```

---

## References

### InfoSec Documentation (Red Hat Internal)
- [rh-pre-commit docs](https://source.redhat.com/departments/strategy_and_operations/it/it_information_security/leaktk/leaktk_components/rh_pre_commit)
- [rh-pre-commit README](https://gitlab.cee.redhat.com/infosec-public/developer-workbench/tools/-/tree/main/rh-pre-commit)
- [rh-pre-commit quickstart](https://gitlab.cee.redhat.com/infosec-public/developer-workbench/tools/-/blob/main/rh-pre-commit/quickstart.sh)

### Public Tools
- [Gitleaks](https://github.com/gitleaks/gitleaks) - Open-source secret scanning
- [pre-commit framework](https://pre-commit.com/) - Hook management framework
