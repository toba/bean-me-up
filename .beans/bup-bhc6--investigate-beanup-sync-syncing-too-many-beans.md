---
# bup-bhc6
title: Investigate beanup sync syncing too many beans
status: in-progress
type: bug
created_at: 2026-01-18T20:51:39Z
updated_at: 2026-01-18T20:51:39Z
---

When running `beanup sync` after changing one bean, it syncs all 467 beans instead of just the changed one. Need to investigate why change detection isn't working properly.