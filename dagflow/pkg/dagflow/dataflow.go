package dagflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oarkflow/bcl"
)

type DataSourceFunc func(context.Context, *DataContext, string) (any, error)

type DataContext struct {
	Engine       *Engine        `json:"-"`
	Workflow     *Workflow      `json:"-"`
	Task         *Task          `json:"task,omitempty"`
	Node         *Node          `json:"node,omitempty"`
	Edge         *Edge          `json:"edge,omitempty"`
	Route        *RouteConfig   `json:"route,omitempty"`
	Input        any            `json:"input,omitempty"`
	Result       any            `json:"result,omitempty"`
	Request      map[string]any `json:"request,omitempty"`
	Session      map[string]any `json:"session,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
	Values       map[string]any `json:"values,omitempty"`
	Services     map[string]any `json:"services,omitempty"`
	Integrations map[string]any `json:"integrations,omitempty"`
	Now          time.Time      `json:"now"`
}

type DataSpec struct {
	Source       string            `json:"source,omitempty"`
	Map          map[string]string `json:"map,omitempty"`
	Set          map[string]any    `json:"set,omitempty"`
	Defaults     map[string]any    `json:"defaults,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Services     map[string]string `json:"services,omitempty"`
	Integrations map[string]string `json:"integrations,omitempty"`
	Pick         []string          `json:"pick,omitempty"`
	Omit         []string          `json:"omit,omitempty"`
	Rename       map[string]string `json:"rename,omitempty"`
	Transforms   []DataTransform   `json:"transforms,omitempty"`
	Filters      []DataFilter      `json:"filters,omitempty"`
	Append       map[string]string `json:"append,omitempty"`
	Prepend      map[string]string `json:"prepend,omitempty"`
	Flatten      []string          `json:"flatten,omitempty"`
	Strict       bool              `json:"strict,omitempty"`
}

type DataTransform struct {
	Field string `json:"field,omitempty"`
	Expr  string `json:"expr,omitempty"`
	Op    string `json:"op,omitempty"`
	Arg   string `json:"arg,omitempty"`
}

type DataFilter struct {
	Expr string `json:"expr,omitempty"`
	Mode string `json:"mode,omitempty"`
}

func (s DataSpec) Empty() bool {
	return s.Source == "" && len(s.Map) == 0 && len(s.Set) == 0 && len(s.Defaults) == 0 && len(s.Env) == 0 && len(s.Services) == 0 && len(s.Integrations) == 0 && len(s.Pick) == 0 && len(s.Omit) == 0 && len(s.Rename) == 0 && len(s.Transforms) == 0 && len(s.Filters) == 0 && len(s.Append) == 0 && len(s.Prepend) == 0 && len(s.Flatten) == 0
}

func (e *Engine) RegisterDataSource(name string, fn DataSourceFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dataSources == nil {
		e.dataSources = map[string]DataSourceFunc{}
	}
	e.dataSources[name] = fn
}

func (e *Engine) applyData(ctx context.Context, spec DataSpec, dc *DataContext, current any) (any, error) {
	if spec.Empty() {
		return current, nil
	}
	if dc == nil {
		dc = &DataContext{}
	}
	dc.Engine = e
	if dc.Now.IsZero() {
		dc.Now = time.Now()
	}
	if dc.Input == nil {
		dc.Input = current
	}
	if dc.Result == nil {
		dc.Result = current
	}

	base := current
	if spec.Source != "" {
		v, err := e.resolveDataValue(ctx, dc, spec.Source)
		if err != nil {
			return nil, err
		}
		base = v
	}
	out := cloneAny(base)
	if len(spec.Pick) > 0 {
		picked := map[string]any{}
		for _, p := range spec.Pick {
			v, ok := getPath(out, p)
			if ok {
				var pv any = picked
				setPath(&pv, p, cloneAny(v))
				picked, _ = pv.(map[string]any)
			} else if spec.Strict {
				return nil, fmt.Errorf("data pick path %q not found", p)
			}
		}
		out = picked
	}
	for _, p := range spec.Omit {
		removePath(out, p)
	}
	for from, to := range spec.Rename {
		v, ok := getPath(out, from)
		if !ok {
			if spec.Strict {
				return nil, fmt.Errorf("data rename path %q not found", from)
			}
			continue
		}
		removePath(out, from)
		setPath(&out, to, v)
	}
	for target, src := range spec.Map {
		v, err := e.resolveDataValue(ctx, dc.withCurrent(out), src)
		if err != nil {
			if spec.Strict {
				return nil, fmt.Errorf("data map %s <- %s: %w", target, src, err)
			}
			continue
		}
		setPath(&out, target, cloneAny(v))
	}
	for target, value := range spec.Set {
		v, err := e.resolveLiteral(ctx, dc.withCurrent(out), value)
		if err != nil {
			return nil, fmt.Errorf("data set %s: %w", target, err)
		}
		setPath(&out, target, v)
	}
	for target, value := range spec.Defaults {
		if _, ok := getPath(out, target); ok {
			continue
		}
		v, err := e.resolveLiteral(ctx, dc.withCurrent(out), value)
		if err != nil {
			return nil, fmt.Errorf("data default %s: %w", target, err)
		}
		setPath(&out, target, v)
	}
	for target, name := range spec.Env {
		setPath(&out, target, os.Getenv(name))
	}
	for target, ref := range spec.Services {
		v, err := e.resolveExternal(ctx, dc.withCurrent(out), "service", ref)
		if err != nil {
			return nil, err
		}
		setPath(&out, target, v)
	}
	for target, ref := range spec.Integrations {
		v, err := e.resolveExternal(ctx, dc.withCurrent(out), "integration", ref)
		if err != nil {
			return nil, err
		}
		setPath(&out, target, v)
	}
	for _, tr := range spec.Transforms {
		v, err := e.applyTransform(ctx, dc.withCurrent(out), tr)
		if err != nil {
			return nil, err
		}
		if tr.Field == "" {
			out = v
		} else {
			setPath(&out, tr.Field, v)
		}
	}
	for field, src := range spec.Append {
		v, err := e.resolveDataValue(ctx, dc.withCurrent(out), src)
		if err != nil {
			return nil, err
		}
		appendPath(&out, field, v, false)
	}
	for field, src := range spec.Prepend {
		v, err := e.resolveDataValue(ctx, dc.withCurrent(out), src)
		if err != nil {
			return nil, err
		}
		appendPath(&out, field, v, true)
	}
	for _, f := range spec.Flatten {
		flattenPath(&out, f)
	}
	for _, flt := range spec.Filters {
		keep, err := e.evalDataFilter(ctx, dc.withCurrent(out), flt)
		if err != nil {
			return nil, err
		}
		if !keep {
			return nil, ErrDataFiltered
		}
	}
	return out, nil
}

var ErrDataFiltered = errors.New("data filtered out")

func (dc *DataContext) withCurrent(v any) *DataContext {
	cp := *dc
	cp.Input = v
	cp.Result = v
	return &cp
}

func (e *Engine) resolveDataValue(ctx context.Context, dc *DataContext, expr string) (any, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" || expr == "." {
		return dc.Input, nil
	}
	if strings.HasPrefix(expr, "=") {
		return evalBCLAny(strings.TrimSpace(strings.TrimPrefix(expr, "=")), e.dataFacts(dc))
	}
	if strings.HasPrefix(expr, "$env.") {
		return os.Getenv(strings.TrimPrefix(expr, "$env.")), nil
	}
	if strings.HasPrefix(expr, "env.") {
		return os.Getenv(strings.TrimPrefix(expr, "env.")), nil
	}
	if strings.HasPrefix(expr, "service.") {
		return e.resolveExternal(ctx, dc, "service", strings.TrimPrefix(expr, "service."))
	}
	if strings.HasPrefix(expr, "integration.") {
		return e.resolveExternal(ctx, dc, "integration", strings.TrimPrefix(expr, "integration."))
	}
	v, ok := getPath(e.dataFacts(dc), expr)
	if ok {
		return v, nil
	}
	v, ok = getPath(dc.Input, expr)
	if ok {
		return v, nil
	}
	return nil, fmt.Errorf("data source path %q not found", expr)
}

func (e *Engine) resolveExternal(ctx context.Context, dc *DataContext, kind, ref string) (any, error) {
	name, key, _ := strings.Cut(ref, ":")
	if name == "" {
		name = ref
	}
	e.mu.RLock()
	fn := e.dataSources[kind+":"+name]
	if fn == nil {
		fn = e.dataSources[name]
	}
	e.mu.RUnlock()
	if fn == nil {
		return nil, fmt.Errorf("%s data source %q is not registered", kind, name)
	}
	return fn(ctx, dc, key)
}

func (e *Engine) resolveLiteral(ctx context.Context, dc *DataContext, v any) (any, error) {
	if s, ok := v.(string); ok {
		if strings.HasPrefix(strings.TrimSpace(s), "=") {
			return e.resolveDataValue(ctx, dc, s)
		}
		if strings.HasPrefix(s, "$env.") || strings.HasPrefix(s, "env.") || strings.HasPrefix(s, "service.") || strings.HasPrefix(s, "integration.") {
			return e.resolveDataValue(ctx, dc, s)
		}
	}
	return cloneAny(v), nil
}

func (e *Engine) applyTransform(ctx context.Context, dc *DataContext, tr DataTransform) (any, error) {
	var v any
	var err error
	if tr.Expr != "" {
		v, err = e.resolveDataValue(ctx, dc, "="+tr.Expr)
	} else if tr.Field != "" {
		v, _ = getPath(dc.Input, tr.Field)
	} else {
		v = dc.Input
	}
	if err != nil {
		return nil, err
	}
	s := fmt.Sprint(v)
	switch strings.ToLower(tr.Op) {
	case "", "expr":
		return v, nil
	case "upper", "uppercase":
		return strings.ToUpper(s), nil
	case "lower", "lowercase":
		return strings.ToLower(s), nil
	case "trim":
		return strings.TrimSpace(s), nil
	case "prefix":
		return tr.Arg + s, nil
	case "suffix":
		return s + tr.Arg, nil
	case "replace":
		old, nw, _ := strings.Cut(tr.Arg, ":")
		return strings.ReplaceAll(s, old, nw), nil
	case "int":
		i, err := strconv.Atoi(s)
		return i, err
	case "float":
		f, err := strconv.ParseFloat(s, 64)
		return f, err
	case "bool":
		b, err := strconv.ParseBool(s)
		return b, err
	case "json":
		var out any
		err := json.Unmarshal([]byte(s), &out)
		return out, err
	case "string":
		return s, nil
	default:
		return nil, fmt.Errorf("unsupported data transform op %q", tr.Op)
	}
}

func (e *Engine) evalDataFilter(ctx context.Context, dc *DataContext, flt DataFilter) (bool, error) {
	if strings.TrimSpace(flt.Expr) == "" {
		return true, nil
	}
	v, err := evalBCLAny(flt.Expr, e.dataFacts(dc))
	if err != nil {
		return false, err
	}
	ok, isBool := v.(bool)
	if !isBool {
		return false, fmt.Errorf("data filter %q did not return bool", flt.Expr)
	}
	if strings.EqualFold(flt.Mode, "drop") || strings.EqualFold(flt.Mode, "exclude") {
		return !ok, nil
	}
	return ok, nil
}

func (e *Engine) dataFacts(dc *DataContext) map[string]any {
	facts := map[string]any{}
	if dc == nil {
		return facts
	}
	facts["input"] = dc.Input
	facts["result"] = dc.Result
	facts["request"] = dc.Request
	facts["session"] = dc.Session
	facts["context"] = dc.Context
	facts["values"] = dc.Values
	facts["services"] = dc.Services
	facts["integrations"] = dc.Integrations
	facts["now"] = dc.Now.Format(time.RFC3339Nano)
	if dc.Task != nil {
		facts["task"] = map[string]any{"id": dc.Task.ID, "workflow_id": dc.Task.WorkflowID, "status": string(dc.Task.Status), "input": dc.Task.Input, "last_result": dc.Task.LastResult, "results": dc.Task.NodeResults, "previous_node": dc.Task.PreviousNode}
	}
	if dc.Workflow != nil {
		facts["workflow"] = map[string]any{"id": dc.Workflow.ID, "version": dc.Workflow.Version, "hash": dc.Workflow.Hash}
	}
	if dc.Node != nil {
		facts["node"] = map[string]any{"id": dc.Node.ID, "type": string(dc.Node.Type), "handler": dc.Node.Handler, "params": dc.Node.Params}
	}
	if dc.Edge != nil {
		facts["edge"] = map[string]any{"id": dc.Edge.ID, "from": dc.Edge.From, "to": dc.Edge.To, "type": string(dc.Edge.Type)}
	}
	if dc.Route != nil {
		facts["route"] = *dc.Route
	}
	return facts
}

func evalBCLAny(expr string, facts map[string]any) (any, error) { return bcl.Eval(expr, facts) }

func cloneAny(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if json.Unmarshal(b, &out) != nil {
		return v
	}
	return out
}

func getPath(root any, path string) (any, bool) {
	path = strings.Trim(strings.TrimSpace(path), ".")
	if path == "" {
		return root, true
	}
	cur := root
	for _, part := range splitPath(path) {
		if part == "" {
			continue
		}
		switch x := cur.(type) {
		case map[string]any:
			v, ok := mapLookupAny(x, part)
			if !ok {
				return nil, false
			}
			cur = v
		case map[string]string:
			v, ok := mapLookupString(x, part)
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(x) {
				return nil, false
			}
			cur = x[i]
		default:
			b, _ := json.Marshal(x)
			var m map[string]any
			if json.Unmarshal(b, &m) != nil {
				return nil, false
			}
			v, ok := mapLookupAny(m, part)
			if !ok {
				return nil, false
			}
			cur = v
		}
	}
	return cur, true
}

func mapLookupAny(m map[string]any, key string) (any, bool) {
	v, ok := m[key]
	if ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

func mapLookupString(m map[string]string, key string) (string, bool) {
	v, ok := m[key]
	if ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

func setPath(root *any, path string, val any) {
	if m, ok := (*root).(map[string]any); ok {
		setPathMap(m, path, val)
		return
	}
	m := map[string]any{}
	if *root != nil {
		m["value"] = *root
	}
	setPathMap(m, path, val)
	*root = m
}
func setPathMap(m map[string]any, path string, val any) {
	parts := splitPath(path)
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = val
			return
		}
		nx, ok := cur[p].(map[string]any)
		if !ok {
			nx = map[string]any{}
			cur[p] = nx
		}
		cur = nx
	}
}
func removePath(root any, path string) {
	m, ok := root.(map[string]any)
	if !ok {
		return
	}
	parts := splitPath(path)
	for i, p := range parts {
		if i == len(parts)-1 {
			delete(m, p)
			return
		}
		nx, ok := m[p].(map[string]any)
		if !ok {
			return
		}
		m = nx
	}
}
func appendPath(root *any, path string, val any, prepend bool) {
	existing, _ := getPath(*root, path)
	arr, _ := existing.([]any)
	if xs, ok := val.([]any); ok {
		if prepend {
			arr = append(xs, arr...)
		} else {
			arr = append(arr, xs...)
		}
	} else if prepend {
		arr = append([]any{val}, arr...)
	} else {
		arr = append(arr, val)
	}
	setPath(root, path, arr)
}
func flattenPath(root *any, path string) {
	v, ok := getPath(*root, path)
	if !ok {
		return
	}
	xs, ok := v.([]any)
	if !ok {
		return
	}
	flat := []any{}
	for _, x := range xs {
		if inner, ok := x.([]any); ok {
			flat = append(flat, inner...)
		} else {
			flat = append(flat, x)
		}
	}
	setPath(root, path, flat)
}
func splitPath(path string) []string {
	path = strings.ReplaceAll(path, "[", ".")
	path = strings.ReplaceAll(path, "]", "")
	return strings.Split(strings.Trim(path, "."), ".")
}
