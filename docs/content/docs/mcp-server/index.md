---
title: "MCP Server"
description: "Run gtasks as a Model Context Protocol (MCP) server so Claude Code, Claude Cowork, and other MCP clients can manage your Google Tasks."
weight: 7
sitemap:
  priority: 0.8
---

## Overview

GTasks can run as a [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server over stdio. This lets MCP-compatible AI clients — such as [Claude Code](https://claude.com/claude-code) and Claude Cowork — view and manage your Google Tasks directly, without you copy-pasting command output.

```bash
gtasks mcp
```

The server speaks JSON-RPC over stdin/stdout. It isn't meant to be run by hand — your MCP client launches it for you.

## Prerequisites

1. **GTasks is installed** and available in your `PATH`.
2. **You're authenticated** — run `gtasks login` once. The server uses the same stored token as the CLI.

## Add to Claude Code

```bash
claude mcp add gtasks -- gtasks mcp
```

Claude Code will start `gtasks mcp` automatically when it needs the tools.

## Add to the Claude Desktop app

1. Open **Settings → Developer → Edit Config**, or edit the file directly:
   - macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
   - Windows: `%APPDATA%\Claude\claude_desktop_config.json`
2. Add a `gtasks` entry under `mcpServers`. The desktop app doesn't use your shell `PATH`, so use the **absolute path** to the `gtasks` binary (find it with `which gtasks`):

   ```json
   {
     "mcpServers": {
       "gtasks": { "command": "/opt/homebrew/bin/gtasks", "args": ["mcp"] }
     }
   }
   ```

3. **Fully quit and reopen** Claude Desktop (Cmd+Q / quit from the tray — closing the window isn't enough).

On macOS you may get a Keychain access prompt the first time the app reads your saved token — click **Always Allow**.

## Add to Claude Cowork / other clients

Add an entry to the client's MCP server configuration:

```json
{
  "mcpServers": {
    "gtasks": { "command": "gtasks", "args": ["mcp"] }
  }
}
```

## Tools

| Tool | Description |
| --- | --- |
| `list_tasklists` | List all task lists. |
| `create_tasklist` | Create a new task list. |
| `delete_tasklist` | Delete a task list (and its tasks). |
| `list_tasks` | List tasks in a list (optionally including completed). |
| `create_task` | Create a task with title, notes, and due date. |
| `update_task` | Update a task's title, notes, or due date. |
| `complete_task` | Mark a task completed. |
| `uncomplete_task` | Mark a completed task as needing action. |
| `delete_task` | Delete a task. |
| `clear_completed_tasks` | Hide all completed tasks in a list. |

### Selecting a task list

Tools that operate on a list accept a `tasklist` argument, which may be either the list **title** or its **id**. If omitted, the configured default list is used (the same `default_task_list` / `GTASKS_DEFAULT_TASKLIST` setting from the Configuration page); if you have only one list, that list is used automatically.

### Due dates

`create_task` and `update_task` accept flexible date formats for `due` (for example `2025-12-25`, `Dec 25`, or `tomorrow`), parsed the same way as the CLI.

## Example interactions

Once connected, you can ask your AI client:

- "Show me my tasks in the Work list"
- "Add a task to Work: Finish the report, due Friday"
- "Mark the 'Finish the report' task as done"
- "Create a new task list called Shopping"
- "Clear completed tasks from my Inbox"
