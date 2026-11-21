package jj

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const operationLogTemplate = `"{\"op\":" ++ json(self) ++ ",\"current\":" ++ json(current_operation) ++ ",\"root\":" ++ json(root) ++ ",\"user\":" ++ json(user) ++ ",\"ago\":" ++ json(time.end().ago()) ++ "}\n"`

const operationLimit = 500

type Operation struct {
	ID          string
	Description string
	Args        string
	User        string
	Time        string
	Snapshot    bool
	Current     bool
	Root        bool
}

type operationLogJSON struct {
	Op struct {
		ID          string            `json:"id"`
		Description string            `json:"description"`
		Snapshot    bool              `json:"is_snapshot"`
		Attributes  map[string]string `json:"attributes"`
	} `json:"op"`
	Current bool   `json:"current"`
	Root    bool   `json:"root"`
	User    string `json:"user"`
	Ago     string `json:"ago"`
}

func (c Client) Operations(ctx context.Context) ([]Operation, error) {
	out, err := c.run(ctx, "operation", "log", "--no-graph", "--limit="+strconv.Itoa(operationLimit), "--config", "ui.log-word-wrap=false", "-T", operationLogTemplate)
	if err != nil {
		return nil, err
	}
	var ops []Operation
	dec := jsontext.NewDecoder(bytes.NewReader(out))
	for {
		var op operationLogJSON
		err := json.UnmarshalDecode(dec, &op)
		if errors.Is(err, io.EOF) {
			return ops, nil
		}
		if err != nil {
			return nil, fmt.Errorf("jj operation log: decode output: %w", err)
		}
		ops = append(ops, Operation{
			ID:          op.Op.ID,
			Description: op.Op.Description,
			Args:        command(op.Op.Attributes["args"]),
			User:        op.User,
			Time:        op.Ago,
			Snapshot:    op.Op.Snapshot,
			Current:     op.Current,
			Root:        op.Root,
		})
	}
}

func command(args string) string {
	if rest, ok := strings.CutPrefix(args, "jj "+globalFlags+" "); ok {
		return "jj " + rest
	}
	return args
}

func (c Client) RestoreOperation(ctx context.Context, id string) error {
	_, err := c.run(ctx, "operation", "restore", "--", id)
	return err
}

func (c Client) RevertOperation(ctx context.Context, id string) error {
	_, err := c.run(ctx, "operation", "revert", "--", id)
	return err
}

func (c Client) AbandonOperation(ctx context.Context, id string) error {
	_, err := c.run(ctx, "operation", "abandon", "--", id)
	return err
}
