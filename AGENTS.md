# Armature — Agent Setup

## Setup

1. **Install `arm`** — build and install the CLI from the repo root:
   ```
   make install
   ```

2. **Deploy bundled skills** — run once per clone to install skills to `.claude/skills/`:
   ```
   arm install-skills
   ```

3. **Register your worker identity** — run before your first task in any clone:
   ```
   arm worker-init
   ```
