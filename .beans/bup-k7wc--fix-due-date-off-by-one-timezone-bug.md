---
# bup-k7wc
title: Fix due date off-by-one timezone bug
status: completed
type: bug
priority: normal
created_at: 2026-02-12T20:29:29Z
updated_at: 2026-02-12T20:31:21Z
---

parseBeanDueDate uses time.Parse which creates dates in UTC. toLocalDateMillis then converts to local time, shifting midnight UTC back one day for timezones behind UTC. Fix: use time.ParseInLocation with time.Local so the date is parsed as local midnight directly.