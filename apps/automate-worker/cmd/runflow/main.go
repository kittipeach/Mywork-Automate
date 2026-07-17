// Command runflow triggers one flow run against a live Temporal server and
// prints the (masked) db.query output — a Docker-free end-to-end demo of the
// interpreter executing db.query against real Postgres. Role defaults to
// "viewer"; pass a role arg to see masking differ (e.g. an exempt role).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"go.temporal.io/sdk/client"

	"github.com/mywork/automate/apps/automate-worker/internal/interpreter"
	"github.com/mywork/automate/apps/automate-worker/internal/worker"
)

func node(id, typ, name string, cfg any) interpreter.NodeDef {
	var raw json.RawMessage
	if cfg != nil {
		raw, _ = json.Marshal(cfg)
	}
	return interpreter.NodeDef{ID: id, Type: typ, Name: name, Config: raw}
}

func main() {
	hostport := os.Getenv("TEMPORAL_HOSTPORT")
	if hostport == "" {
		hostport = "127.0.0.1:7233"
	}
	role := "viewer"
	if len(os.Args) > 1 {
		role = os.Args[1]
	}

	c, err := client.Dial(client.Options{HostPort: hostport, Namespace: "automate"})
	if err != nil {
		log.Fatalf("dial temporal: %v", err)
	}
	defer c.Close()

	// trigger.manual -> db.query -> logic.if(rowCount>0) --true--> deliver(noop)
	flow := interpreter.FlowDef{
		Nodes: []interpreter.NodeDef{
			node("t1", "trigger.manual", "Manual trigger", nil),
			node("q1", "db.query", "Query employees", map[string]any{
				"sql":     "SELECT name, salary, citizen_id, phone, email FROM employees ORDER BY id",
				"maxRows": 50,
			}),
			node("if1", "logic.if", "Has rows?", map[string]any{"left": "rowCount", "op": ">", "right": 0}),
			node("done", "noop", "Deliver", nil),
		},
		Edges: []interpreter.EdgeDef{
			{Source: "t1", Target: "q1"},
			{Source: "q1", Target: "if1"},
			{Source: "if1", Target: "done", Label: "true"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	we, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{TaskQueue: worker.TaskQueue},
		interpreter.FlowWorkflow, interpreter.FlowInput{Flow: flow, ViewerRoles: []string{role}})
	if err != nil {
		log.Fatalf("start workflow: %v", err)
	}
	fmt.Printf("▶ started workflow %s (viewer role=%q)\n", we.GetID(), role)

	var res interpreter.FlowResult
	if err := we.Get(ctx, &res); err != nil {
		log.Fatalf("workflow failed: %v", err)
	}

	fmt.Printf("✓ path executed: %v\n", res.Path)
	q := res.Outputs["q1"]
	fmt.Printf("  db.query rowCount=%v truncated=%v\n", q.Meta["rowCount"], q.Meta["truncated"])
	b, _ := json.MarshalIndent(q.Items, "  ", "  ")
	fmt.Printf("  db.query output (masked for role %q):\n  %s\n", role, b)
	if len(res.Path) == 4 {
		fmt.Println("  branch: logic.if rowCount>0 = true → Deliver ran (full path)")
	}
}
