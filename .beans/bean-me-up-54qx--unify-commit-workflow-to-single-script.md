---
# bean-me-up-54qx
title: Unify commit workflow to single script
status: completed
type: task
priority: normal
created_at: 2026-01-17T21:35:02Z
updated_at: 2026-01-17T21:40:13Z
---

Convert the two-phase commit workflow (pre/post) to a single unified script that:
- Takes subject, description, and optional version as arguments
- Does everything in one command: lint, test, gitignore check, stage, commit, sync beans, push
- Optionally creates GitHub release when version provided

## Checklist
- [x] Rewrite scripts/commit.sh as single unified script
- [x] Simplify SKILL.md workflow instructions
- [x] Test the workflow