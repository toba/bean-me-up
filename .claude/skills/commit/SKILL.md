---
name: commit
description: Stage all changes and commit with a descriptive message. Use when the user asks to commit, save changes, or says "/commit".
args: "[push]"
---

## Workflow

**IMPORTANT**: Only use `PUSH=true` when the user explicitly says "/commit push" or asks to push. Plain "/commit" should NEVER push.

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
   # Local commit only (no push, no release):
   .claude/skills/commit/scripts/commit.sh "subject" "description"

   # Push and release with version bump:
   PUSH=true .claude/skills/commit/scripts/commit.sh "subject" "description" vX.Y.Z

   # Push without version bump:
   PUSH=true .claude/skills/commit/scripts/commit.sh "subject" "description"

   # Subject only (no description):
   .claude/skills/commit/scripts/commit.sh "subject" ""
   ```

The script handles: lint, test, gitignore check, stage, commit, and sync beans. Push and release only happen when `PUSH=true`.
