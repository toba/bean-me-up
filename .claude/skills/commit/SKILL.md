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
