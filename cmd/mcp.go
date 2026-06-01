package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/araddon/dateparse"
	"github.com/fatih/color"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pmunin/gtasks-cli/api"
	"github.com/pmunin/gtasks-cli/internal/config"
	"github.com/spf13/cobra"
	gtasks "google.golang.org/api/tasks/v1"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run gtasks as an MCP server (stdio)",
	Long: `Run gtasks as a Model Context Protocol (MCP) server over stdio.

This exposes your Google Tasks to MCP-compatible AI clients such as
Claude Code and Claude Cowork, so the assistant can view and manage
your tasks and task lists directly.

You must be logged in first:
  gtasks login

Add to Claude Code:
  claude mcp add gtasks -- gtasks mcp

Add to Claude Cowork / other clients, add an entry to the MCP config:
  {
    "mcpServers": {
      "gtasks": { "command": "gtasks", "args": ["mcp"] }
    }
  }

The server speaks JSON-RPC over stdin/stdout; do not run it interactively.`,
	RunE:          runMCP,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	// stdout is reserved for the JSON-RPC protocol stream — route any stray
	// colored/log output to stderr so it cannot corrupt the protocol.
	color.Output = os.Stderr

	srv, err := api.GetService()
	if err != nil {
		return fmt.Errorf("not authenticated: run 'gtasks login' first (%v)", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "gtasks",
		Version: Version,
	}, nil)

	registerTaskTools(server, srv)

	err = server.Run(context.Background(), &mcp.StdioTransport{})
	// A closed stdin (client disconnect) is a normal shutdown, not an error.
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// ---- Tool I/O types ----------------------------------------------------------

type taskListItem struct {
	ID    string `json:"id" jsonschema:"the task list id"`
	Title string `json:"title" jsonschema:"the task list title"`
}

type taskItem struct {
	ID          string `json:"id" jsonschema:"the task id"`
	Title       string `json:"title" jsonschema:"the task title"`
	Notes       string `json:"notes,omitempty" jsonschema:"freeform notes / description"`
	Status      string `json:"status" jsonschema:"needsAction or completed"`
	Due         string `json:"due,omitempty" jsonschema:"due date in RFC3339 (date-only is respected by Google Tasks)"`
	Completed   string `json:"completed,omitempty" jsonschema:"completion timestamp in RFC3339, if completed"`
	Position    string `json:"position,omitempty" jsonschema:"sort position within the list"`
	Parent      string `json:"parent,omitempty" jsonschema:"parent task id for subtasks"`
	WebViewLink string `json:"webViewLink,omitempty" jsonschema:"link to open the task in Google Tasks"`
}

type listTaskListsOut struct {
	TaskLists []taskListItem `json:"taskLists" jsonschema:"the available task lists"`
}

type tasklistSelector struct {
	TaskList string `json:"tasklist,omitempty" jsonschema:"task list title or id; if omitted, the configured default (or the only list) is used"`
}

type listTasksIn struct {
	tasklistSelector
	IncludeCompleted bool `json:"includeCompleted,omitempty" jsonschema:"include completed tasks (default false)"`
	Max              int  `json:"max,omitempty" jsonschema:"maximum number of tasks to return (0 = all)"`
}

type listTasksOut struct {
	TaskList string     `json:"taskList" jsonschema:"the resolved task list title"`
	Tasks    []taskItem `json:"tasks" jsonschema:"the tasks in the list"`
}

type createTaskIn struct {
	tasklistSelector
	Title string `json:"title" jsonschema:"the task title (required)"`
	Notes string `json:"notes,omitempty" jsonschema:"freeform notes / description"`
	Due   string `json:"due,omitempty" jsonschema:"due date, flexible format e.g. '2025-12-25', 'Dec 25', 'tomorrow'"`
}

type updateTaskIn struct {
	tasklistSelector
	TaskID string  `json:"taskId" jsonschema:"the id of the task to update (required)"`
	Title  *string `json:"title,omitempty" jsonschema:"new title; omit to keep current"`
	Notes  *string `json:"notes,omitempty" jsonschema:"new notes; omit to keep current, empty string to clear"`
	Due    *string `json:"due,omitempty" jsonschema:"new due date; omit to keep current, empty string to clear"`
}

type taskRefIn struct {
	tasklistSelector
	TaskID string `json:"taskId" jsonschema:"the id of the task (required)"`
}

type taskResultOut struct {
	Task taskItem `json:"task" jsonschema:"the affected task"`
}

type messageOut struct {
	Message string `json:"message" jsonschema:"a human-readable result message"`
}

type createTaskListIn struct {
	Title string `json:"title" jsonschema:"the title of the new task list (required)"`
}

type deleteTaskListIn struct {
	TaskList string `json:"tasklist" jsonschema:"title or id of the task list to delete (required)"`
}

// ---- Registration ------------------------------------------------------------

func registerTaskTools(server *mcp.Server, srv *gtasks.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tasklists",
		Description: "List all Google Tasks task lists for the signed-in account.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listTaskListsOut, error) {
		lists, err := getTaskListsRaw(srv)
		if err != nil {
			return nil, listTaskListsOut{}, err
		}
		out := listTaskListsOut{TaskLists: make([]taskListItem, 0, len(lists))}
		for _, l := range lists {
			out.TaskLists = append(out.TaskLists, taskListItem{ID: l.Id, Title: l.Title})
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks in a task list. If 'tasklist' is omitted, uses the configured default list (or the only list).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listTasksIn) (*mcp.CallToolResult, listTasksOut, error) {
		tl, err := resolveTaskList(srv, in.TaskList)
		if err != nil {
			return nil, listTasksOut{}, err
		}
		items, err := api.GetTasks(srv, tl.Id, in.IncludeCompleted, in.Max)
		if err != nil {
			if err.Error() == "no Tasks found" {
				return nil, listTasksOut{TaskList: tl.Title, Tasks: []taskItem{}}, nil
			}
			return nil, listTasksOut{}, err
		}
		out := listTasksOut{TaskList: tl.Title, Tasks: make([]taskItem, 0, len(items))}
		for _, t := range items {
			out.Tasks = append(out.Tasks, toTaskItem(t))
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_task",
		Description: "Create a new task in a task list.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createTaskIn) (*mcp.CallToolResult, taskResultOut, error) {
		if in.Title == "" {
			return nil, taskResultOut{}, fmt.Errorf("title is required")
		}
		tl, err := resolveTaskList(srv, in.TaskList)
		if err != nil {
			return nil, taskResultOut{}, err
		}
		t := &gtasks.Task{Title: in.Title, Notes: in.Notes}
		if in.Due != "" {
			due, err := parseDue(in.Due)
			if err != nil {
				return nil, taskResultOut{}, err
			}
			t.Due = due
		}
		created, err := api.CreateTask(srv, t, tl.Id)
		if err != nil {
			return nil, taskResultOut{}, err
		}
		return nil, taskResultOut{Task: toTaskItem(created)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_task",
		Description: "Update an existing task's title, notes, or due date. Only provided fields are changed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in updateTaskIn) (*mcp.CallToolResult, taskResultOut, error) {
		if in.TaskID == "" {
			return nil, taskResultOut{}, fmt.Errorf("taskId is required")
		}
		tl, err := resolveTaskList(srv, in.TaskList)
		if err != nil {
			return nil, taskResultOut{}, err
		}
		t, err := api.GetTaskInfo(srv, tl.Id, in.TaskID)
		if err != nil {
			return nil, taskResultOut{}, fmt.Errorf("task not found: %v", err)
		}
		if in.Title != nil {
			t.Title = *in.Title
		}
		if in.Notes != nil {
			t.Notes = *in.Notes
		}
		if in.Due != nil {
			if *in.Due == "" {
				t.Due = ""
				t.ForceSendFields = append(t.ForceSendFields, "Due")
			} else {
				due, err := parseDue(*in.Due)
				if err != nil {
					return nil, taskResultOut{}, err
				}
				t.Due = due
			}
		}
		updated, err := api.UpdateTask(srv, t, tl.Id)
		if err != nil {
			return nil, taskResultOut{}, err
		}
		return nil, taskResultOut{Task: toTaskItem(updated)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "complete_task",
		Description: "Mark a task as completed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in taskRefIn) (*mcp.CallToolResult, taskResultOut, error) {
		return setTaskStatus(srv, in, "completed")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "uncomplete_task",
		Description: "Mark a completed task as needing action again.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in taskRefIn) (*mcp.CallToolResult, taskResultOut, error) {
		return setTaskStatus(srv, in, "needsAction")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_task",
		Description: "Delete a task from a task list.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in taskRefIn) (*mcp.CallToolResult, messageOut, error) {
		if in.TaskID == "" {
			return nil, messageOut{}, fmt.Errorf("taskId is required")
		}
		tl, err := resolveTaskList(srv, in.TaskList)
		if err != nil {
			return nil, messageOut{}, err
		}
		if err := api.DeleteTask(srv, in.TaskID, tl.Id); err != nil {
			return nil, messageOut{}, err
		}
		return nil, messageOut{Message: fmt.Sprintf("deleted task %s from %q", in.TaskID, tl.Title)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "clear_completed_tasks",
		Description: "Hide all completed tasks in a task list so they are no longer returned.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in tasklistSelector) (*mcp.CallToolResult, messageOut, error) {
		tl, err := resolveTaskList(srv, in.TaskList)
		if err != nil {
			return nil, messageOut{}, err
		}
		if err := api.ClearTasks(srv, tl.Id); err != nil {
			return nil, messageOut{}, err
		}
		return nil, messageOut{Message: fmt.Sprintf("cleared completed tasks from %q", tl.Title)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_tasklist",
		Description: "Create a new task list.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createTaskListIn) (*mcp.CallToolResult, taskListItem, error) {
		if in.Title == "" {
			return nil, taskListItem{}, fmt.Errorf("title is required")
		}
		created, err := srv.Tasklists.Insert(&gtasks.TaskList{Title: in.Title}).Do()
		if err != nil {
			return nil, taskListItem{}, err
		}
		return nil, taskListItem{ID: created.Id, Title: created.Title}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_tasklist",
		Description: "Delete a task list (and all its tasks). This cannot be undone.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deleteTaskListIn) (*mcp.CallToolResult, messageOut, error) {
		if in.TaskList == "" {
			return nil, messageOut{}, fmt.Errorf("tasklist is required")
		}
		tl, err := resolveTaskList(srv, in.TaskList)
		if err != nil {
			return nil, messageOut{}, err
		}
		if err := api.DeleteTaskList(srv, tl.Id); err != nil {
			return nil, messageOut{}, err
		}
		return nil, messageOut{Message: fmt.Sprintf("deleted task list %q", tl.Title)}, nil
	})
}

// ---- Helpers -----------------------------------------------------------------

func setTaskStatus(srv *gtasks.Service, in taskRefIn, status string) (*mcp.CallToolResult, taskResultOut, error) {
	if in.TaskID == "" {
		return nil, taskResultOut{}, fmt.Errorf("taskId is required")
	}
	tl, err := resolveTaskList(srv, in.TaskList)
	if err != nil {
		return nil, taskResultOut{}, err
	}
	t, err := api.GetTaskInfo(srv, tl.Id, in.TaskID)
	if err != nil {
		return nil, taskResultOut{}, fmt.Errorf("task not found: %v", err)
	}
	t.Status = status
	if status == "needsAction" {
		t.Completed = nil
		t.ForceSendFields = append(t.ForceSendFields, "Completed")
	}
	updated, err := api.UpdateTask(srv, t, tl.Id)
	if err != nil {
		return nil, taskResultOut{}, err
	}
	return nil, taskResultOut{Task: toTaskItem(updated)}, nil
}

// getTaskListsRaw lists task lists without the os.Exit behaviour of api.GetTaskLists,
// so a transient error can be returned as an MCP error instead of killing the server.
func getTaskListsRaw(srv *gtasks.Service) ([]*gtasks.TaskList, error) {
	r, err := srv.Tasklists.List().MaxResults(100).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve task lists: %v", err)
	}
	return r.Items, nil
}

// resolveTaskList finds a task list by title or id. If nameOrID is empty it uses
// the configured default; if none and only one list exists, that one is used.
func resolveTaskList(srv *gtasks.Service, nameOrID string) (*gtasks.TaskList, error) {
	lists, err := getTaskListsRaw(srv)
	if err != nil {
		return nil, err
	}
	if len(lists) == 0 {
		return nil, fmt.Errorf("no task lists found")
	}

	effective := nameOrID
	if effective == "" {
		effective = config.GetDefaultTaskList()
	}

	if effective == "" {
		if len(lists) == 1 {
			return lists[0], nil
		}
		var titles []string
		for _, l := range lists {
			titles = append(titles, l.Title)
		}
		return nil, fmt.Errorf("multiple task lists exist; specify 'tasklist' (one of: %v)", titles)
	}

	for _, l := range lists {
		if l.Id == effective || l.Title == effective {
			return l, nil
		}
	}
	return nil, fmt.Errorf("task list not found: %q", effective)
}

func parseDue(input string) (string, error) {
	t, err := dateparse.ParseAny(input)
	if err != nil {
		return "", fmt.Errorf("date format incorrect: %q (examples: https://github.com/araddon/dateparse#extended-example)", input)
	}
	return t.Format(time.RFC3339), nil
}

func toTaskItem(t *gtasks.Task) taskItem {
	item := taskItem{
		ID:          t.Id,
		Title:       t.Title,
		Notes:       t.Notes,
		Status:      t.Status,
		Due:         t.Due,
		Position:    t.Position,
		Parent:      t.Parent,
		WebViewLink: t.WebViewLink,
	}
	if t.Completed != nil {
		item.Completed = *t.Completed
	}
	return item
}
