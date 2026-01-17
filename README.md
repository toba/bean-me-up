# 🫘 bean-me-up

Sync [beans](https://github.com/hmans/beans) to ClickUp tasks.

## Overview

bean-me-up is a companion tool for the [beans](https://github.com/hmans/beans) issue tracker that syncs beans to ClickUp tasks. It:

- Calls the standard `beans` CLI with `--json` output (no internal library dependency)
- Stores sync state in `.beans/.sync.json` (never modifies bean files)
- Works alongside standard beans without modification

## Installation

```bash
go install github.com/STR-Consulting/bean-me-up/cmd/beanup@latest
```

Or build from source:

```bash
git clone https://github.com/STR-Consulting/bean-me-up
cd bean-me-up
go build ./cmd/beanup
```

## Setup

1. Create a `.beans.clickup.yml` configuration file in your project:

```yaml
beans:
  clickup:
    list_id: "123456789"          # Required: ClickUp list ID
    assignee: 12345               # Optional: default assignee user ID
    status_mapping:               # Optional: custom status mappings
      draft: "backlog"
      todo: "to do"
      in-progress: "in progress"
      completed: "complete"
    custom_fields:                # Optional: map bean fields to ClickUp fields
      bean_id: "field-uuid"
      created_at: "field-uuid"
      updated_at: "field-uuid"
    users:                        # Optional: for @mention support
      jason: 12345
    sync_filter:
      exclude_status:
        - scrapped
```

2. Set the `CLICKUP_TOKEN` environment variable:

```bash
export CLICKUP_TOKEN="pk_your_clickup_api_token"
```

3. Find your ClickUp list ID from the URL when viewing the list, or use the ClickUp API.

4. Use helper commands to discover configuration values:

```bash
# List available statuses on your ClickUp list
beanup statuses

# List custom fields and their IDs
beanup fields

# List workspace members and their IDs
beanup users
```

## Usage

### Sync Beans to ClickUp

```bash
# Sync all beans (respects sync_filter)
beanup sync

# Sync specific beans
beanup sync bean-abc1 bean-def2

# Preview what would be synced
beanup sync --dry-run

# Force update even if unchanged
beanup sync --force

# Skip relationship syncing (dependencies)
beanup sync --no-relationships
```

### Manual Linking

```bash
# Link a bean to an existing ClickUp task
beanup link bean-abc1 868h4abcd

# Remove a link
beanup unlink bean-abc1
```

### View Status

```bash
# Show sync status for all linked beans
beanup status

# Show status for specific beans
beanup status bean-abc1 bean-def2

# JSON output
beanup status --json
```

## How Sync Works

1. **New beans** create new ClickUp tasks with:
   - Title and description from the bean
   - Status mapped according to `status_mapping`
   - Priority mapped (critical→Urgent, high→High, etc.)
   - Parent/subtask relationships if the parent bean is also synced
   - Custom fields if configured

2. **Existing beans** update their linked ClickUp tasks when:
   - The bean's `updated_at` is newer than `synced_at`
   - Or `--force` is used

3. **Relationships** are synced as ClickUp dependencies:
   - Bean A `blocking: [B, C]` → Tasks B and C depend on task A

4. **Sync state** is stored in `.beans/.sync.json` (not in bean files):
   ```json
   {
     "beans": {
       "bean-abc1": {
         "clickup": {
           "task_id": "868h4abcd",
           "synced_at": "2024-01-15T10:30:00Z"
         }
       }
     }
   }
   ```
   This avoids conflicts with the beans CLI which overwrites frontmatter.

## Configuration Reference

The configuration file uses a nested structure under `beans.clickup`. The beans path is read from `.beans.yml` (the beans CLI configuration).

### `beans.clickup.list_id`

Required. The ClickUp list ID to sync tasks to.

### `beans.clickup.assignee`

Optional. ClickUp user ID to assign new tasks to. If not set, tasks are assigned to the API token owner. Set to `0` for unassigned tasks.

### `beans.clickup.status_mapping`

Map bean statuses to ClickUp status names. Defaults:

```yaml
status_mapping:
  draft: "backlog"
  todo: "to do"
  in-progress: "in progress"
  completed: "complete"
  scrapped: "closed"
```

### `beans.clickup.priority_mapping`

Map bean priorities to ClickUp priority values (1=Urgent, 2=High, 3=Normal, 4=Low). Defaults:

```yaml
priority_mapping:
  critical: 1
  high: 2
  normal: 3
  low: 4
  deferred: 4
```

### `beans.clickup.custom_fields`

Map bean fields to ClickUp custom field UUIDs:

```yaml
custom_fields:
  bean_id: "uuid"      # Text field for bean ID
  created_at: "uuid"   # Date field for creation time
  updated_at: "uuid"   # Date field for last update
```

### `beans.clickup.users`

Map usernames to ClickUp user IDs for @mention support:

```yaml
users:
  jason: 12345
  sarah: 67890
```

When syncing, @mentions in bean bodies create a comment tagging the users.

### `beans.clickup.sync_filter`

Control which beans are synced:

```yaml
sync_filter:
  exclude_status: ["scrapped", "completed"]
```

## Attribution

This project syncs with [beans](https://github.com/hmans/beans), an agentic-first issue tracker by [hmans](https://github.com/hmans).

## License

MIT
