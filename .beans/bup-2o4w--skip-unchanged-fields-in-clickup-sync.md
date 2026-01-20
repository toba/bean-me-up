---
# bup-2o4w
title: Skip unchanged fields in ClickUp sync
status: completed
type: feature
priority: normal
created_at: 2026-01-20T23:46:47Z
updated_at: 2026-01-20T23:48:33Z
---

When syncing beans to ClickUp, compare fields against current task state and only include changed fields in updates. This prevents noisy activity logs showing 'You changed X from Y to Y' for unchanged fields.