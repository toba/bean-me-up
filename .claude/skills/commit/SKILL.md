---
name: commit
description: Stage all changes and commit with a descriptive message. Use when the user asks to commit, save changes, or says "/commit".
---

## Workflow

1. Run pre-commit checks:
   ```bash
   golangci-lint run && go test ./...
   ```
   If checks fail, report issues and STOP.

2. Stage and review changes:
   ```bash
   git add -A
   git status --short
   git diff --staged
   ```

3. Create commit with concise, descriptive message:
   - Lowercase, imperative mood (e.g., "add feature" not "Added feature")
   - Focus on "why" not just "what"
   - End with: `Co-Authored-By: Claude <noreply@anthropic.com>`

4. Verify: `git status`

5. Determine version bump (if any tags exist, otherwise skip):
   - Check current version: `git describe --tags --abbrev=0 2>/dev/null || echo "none"`
   - Analyze the committed changes:
     - **Major (X.0.0)**: Breaking changes - removed/renamed public APIs, changed behavior
     - **Minor (0.X.0)**: New features - new rules, new CLI flags, new capabilities
     - **Patch (0.0.X)**: Bug fixes, docs, refactoring, dependency updates
   - If version bump warranted, create annotated tag:
     ```bash
     git tag -a vX.Y.Z -m "Release vX.Y.Z"
     ```
   - Report: "Tagged vX.Y.Z" or "No version bump needed"
