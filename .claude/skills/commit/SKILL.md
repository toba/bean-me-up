---
name: commit
description: Stage all changes and commit with a descriptive message. Use when the user asks to commit, save changes, or says "/commit".
---

## Workflow

1. Review changes and current version:
   ```bash
   git diff
   git describe --tags --abbrev=0 2>/dev/null || echo "no tags yet"
   ```

2. Analyze staged changes for version bump:
   - **Major (X.0.0)**: Breaking changes - removed/renamed public APIs, changed behavior
   - **Minor (0.X.0)**: New features - new capabilities, new CLI flags
   - **Patch (0.0.X)**: Bug fixes, docs, refactoring, dependency updates

3. Craft commit message:
   - Subject: lowercase, imperative mood (e.g., "add feature" not "Added feature")
   - Description: focus on "why" not just "what"

4. Run commit script:
   ```bash
   # Without version bump:
   .claude/skills/commit/scripts/commit.sh "subject" "description"

   # With version bump:
   .claude/skills/commit/scripts/commit.sh "subject" "description" vX.Y.Z

   # Subject only (no description):
   .claude/skills/commit/scripts/commit.sh "subject" ""
   ```

The script handles everything: lint, test, gitignore check, stage, commit, sync beans, push, and release (if version provided).
