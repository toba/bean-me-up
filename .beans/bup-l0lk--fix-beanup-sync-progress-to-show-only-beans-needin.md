---
# bup-l0lk
title: Fix beanup sync progress to show only beans needing sync
status: completed
type: task
priority: normal
created_at: 2026-01-18T21:00:36Z
updated_at: 2026-01-18T21:01:19Z
---

Pre-compute which beans need syncing by comparing timestamps upfront, then only process and show progress for those beans.