---
# bup-onq7
title: Bean type not correctly mapping to ClickUp task types
status: completed
type: bug
priority: normal
created_at: 2026-01-18T17:23:39Z
updated_at: 2026-01-18T17:35:06Z
---

Bean type (e.g., bug) is not being correctly mapped to ClickUp task types. For example, bean-me-up-1xa7 is a bug but in ClickUp it became a regular task.

ClickUp uses `custom_item_id` in the create task request to set task type. Need to:
1. Fetch custom task types from ClickUp API (`GET /team/{team_id}/custom_item`)
2. Add `type_mapping` config to map bean types to ClickUp custom item IDs
3. Add `custom_item_id` field to CreateTaskRequest
4. Apply the type mapping when creating/updating tasks
5. Add `beanup types` command to list available task types for configuration

## Checklist

- [x] Add GetCustomItems API method to fetch available task types
- [x] Add `beanup types` command to display available task types
- [x] Add `type_mapping` config option
- [x] Add `custom_item_id` field to CreateTaskRequest
- [x] Apply type mapping in sync logic
- [x] Update `beanup init` command to include type_mapping in generated config
- [x] Update README with type_mapping documentation