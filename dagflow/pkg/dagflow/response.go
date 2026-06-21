package dagflow

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

const normalizeJSONMaxDepth = 64

type PublicTaskReceipt struct {
	Accepted    bool       `json:"accepted"`
	TaskID      string     `json:"task_id"`
	WorkflowID  string     `json:"workflow_id"`
	Status      TaskStatus `json:"status"`
	CurrentNode string     `json:"current_node,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StatusURL   string     `json:"status_url,omitempty"`
	AuditURL    string     `json:"audit_url,omitempty"`
}

type PublicTaskState struct {
	TaskID        string     `json:"task_id"`
	WorkflowID    string     `json:"workflow_id"`
	Status        TaskStatus `json:"status"`
	CurrentNode   string     `json:"current_node,omitempty"`
	WaitingNodeID string     `json:"waiting_node_id,omitempty"`
	ResumeToken   string     `json:"resume_token,omitempty"`
	Result        any        `json:"result,omitempty"`
	Error         string     `json:"error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

type PublicChainReceipt struct {
	Accepted    bool       `json:"accepted"`
	ChainRunID  string     `json:"chain_run_id"`
	ChainID     string     `json:"chain_id"`
	WorkflowIDs []string   `json:"workflow_ids"`
	Status      TaskStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StatusURL   string     `json:"status_url,omitempty"`
}

type PublicChainState struct {
	ChainRunID  string     `json:"chain_run_id"`
	ChainID     string     `json:"chain_id"`
	WorkflowIDs []string   `json:"workflow_ids"`
	Status      TaskStatus `json:"status"`
	Result      any        `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type TaskActivitySummary struct {
	TaskID        string            `json:"task_id"`
	WorkflowID    string            `json:"workflow_id"`
	Status        TaskStatus        `json:"status"`
	CurrentNode   string            `json:"current_node,omitempty"`
	TotalEvents   int               `json:"total_events"`
	TotalErrors   int               `json:"total_errors"`
	NodeStatus    map[string]string `json:"node_status"`
	StartedAt     time.Time         `json:"started_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	CompletedAt   *time.Time        `json:"completed_at,omitempty"`
	LastEventTime *time.Time        `json:"last_event_time,omitempty"`
}

// publicResult normalizes values into JSON-compatible Go values.
// It does not force object-only output because ops/debug endpoints may need
// to expose arrays/scalars inside structured envelopes.
func publicResult(v any) any {
	return normalizeJSONValue(v)
}

// publicPayload returns only client-safe business data.
//
// Public route responses are always a JSON object or an array of objects.
// Raw strings, nil, numbers, booleans, and arrays containing scalars are wrapped
// so the HTTP layer never emits accidental stringified maps, null-only responses,
// or scalar API responses.
func publicPayload(v any) any {
	return ensureObjectOrObjectArray(publicResult(v))
}

func publicTaskResult(t *Task) any {
	if t == nil {
		return nil
	}
	return publicResult(t.Result)
}

func publicTaskReceipt(t *Task) PublicTaskReceipt {
	if t == nil {
		return PublicTaskReceipt{}
	}

	return PublicTaskReceipt{
		Accepted:    true,
		TaskID:      t.ID,
		WorkflowID:  t.WorkflowID,
		Status:      t.Status,
		CurrentNode: t.CurrentNode,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		StatusURL:   "/ops/tasks/" + t.ID,
		AuditURL:    "/ops/tasks/" + t.ID + "/activities",
	}
}

func publicTaskState(t *Task) PublicTaskState {
	if t == nil {
		return PublicTaskState{}
	}

	return PublicTaskState{
		TaskID:        t.ID,
		WorkflowID:    t.WorkflowID,
		Status:        t.Status,
		CurrentNode:   t.CurrentNode,
		WaitingNodeID: t.WaitingNodeID,
		ResumeToken:   t.ResumeToken,
		Result:        publicTaskResult(t),
		Error:         t.Error,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
		CompletedAt:   t.CompletedAt,
	}
}

func publicChainReceipt(r *ChainRun) PublicChainReceipt {
	if r == nil {
		return PublicChainReceipt{}
	}

	return PublicChainReceipt{
		Accepted:    true,
		ChainRunID:  r.ID,
		ChainID:     r.ChainID,
		WorkflowIDs: append([]string(nil), r.WorkflowIDs...),
		Status:      r.Status,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		StatusURL:   "/ops/chains/" + r.ID,
	}
}

func publicChainState(r *ChainRun) PublicChainState {
	if r == nil {
		return PublicChainState{}
	}

	return PublicChainState{
		ChainRunID:  r.ID,
		ChainID:     r.ChainID,
		WorkflowIDs: append([]string(nil), r.WorkflowIDs...),
		Status:      r.Status,
		Result:      publicResult(r.Result),
		Error:       r.Error,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		CompletedAt: r.CompletedAt,
	}
}

func taskActivitySummary(t *Task) TaskActivitySummary {
	out := TaskActivitySummary{
		NodeStatus: map[string]string{},
	}

	if t == nil {
		return out
	}

	out.TaskID = t.ID
	out.WorkflowID = t.WorkflowID
	out.Status = t.Status
	out.CurrentNode = t.CurrentNode
	out.TotalEvents = len(t.Audit)
	out.TotalErrors = len(t.Errors)
	out.StartedAt = t.CreatedAt
	out.UpdatedAt = t.UpdatedAt
	out.CompletedAt = t.CompletedAt

	for id, st := range t.NodeStates {
		if st != nil {
			out.NodeStatus[id] = string(st.Status)
		}
	}

	if len(t.Audit) > 0 {
		at := t.Audit[len(t.Audit)-1].At
		out.LastEventTime = &at
	}

	return out
}

func normalizeJSONValue(v any) any {
	return normalizeJSONValueDepth(v, 0)
}

func normalizeJSONValueDepth(v any, depth int) any {
	if depth > normalizeJSONMaxDepth {
		return map[string]any{
			"error": "response normalization depth exceeded",
		}
	}

	switch x := v.(type) {
	case nil:
		return nil

	case bool:
		return x

	case int:
		return x
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x

	case uint:
		return x
	case uint8:
		return uint64(x)
	case uint16:
		return uint64(x)
	case uint32:
		return uint64(x)
	case uint64:
		return x

	case float32:
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
		return x

	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return strconv.FormatFloat(x, 'g', -1, 64)
		}
		return x

	case json.Number:
		return x

	case string:
		return parseJSONStringValueDepth(x, depth+1)

	case []byte:
		return parseJSONStringValueDepth(string(x), depth+1)

	case json.RawMessage:
		return parseJSONStringValueDepth(string(x), depth+1)

	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[k] = normalizeJSONValueDepth(v, depth+1)
		}
		return out

	case map[string]string:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[k] = normalizeJSONValueDepth(v, depth+1)
		}
		return out

	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = normalizeJSONValueDepth(v, depth+1)
		}
		return out

	case []string:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = normalizeJSONValueDepth(v, depth+1)
		}
		return out

	default:
		b, err := json.Marshal(x)
		if err != nil {
			return x
		}

		var out any
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.UseNumber()

		if err := dec.Decode(&out); err != nil {
			return x
		}

		return normalizeJSONValueDepth(out, depth+1)
	}
}

func parseJSONStringValue(s string) any {
	return parseJSONStringValueDepth(s, 0)
}

func parseJSONStringValueDepth(s string, depth int) any {
	if depth > normalizeJSONMaxDepth {
		return s
	}

	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return s
	}

	first := trimmed[0]

	// Only object/array-looking strings should be parsed.
	if first != '{' && first != '[' {
		return s
	}

	// Must be a complete top-level object/array. Otherwise it is just a string.
	if !hasBalancedInspectContainer(trimmed) {
		return s
	}

	var out any
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()

	if err := dec.Decode(&out); err == nil {
		return normalizeJSONValueDepth(out, depth+1)
	}

	parsed, ok := parseInspectValue(trimmed)
	if !ok {
		return s
	}

	if parsedString, ok := parsed.(string); ok && parsedString == trimmed {
		return s
	}

	return normalizeJSONValueDepth(parsed, depth+1)
}

func ensureObjectOrObjectArray(v any) any {
	v = normalizeJSONValueDepth(v, 0)

	switch x := v.(type) {
	case map[string]any:
		return x

	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			item = normalizeJSONValueDepth(item, 1)

			if obj, ok := item.(map[string]any); ok {
				out[i] = obj
				continue
			}

			out[i] = map[string]any{
				"data": item,
			}
		}
		return out

	default:
		return map[string]any{
			"data": x,
		}
	}
}

func parseInspectValue(s string) (any, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}

	// Object-like value only when the WHOLE value is wrapped by balanced braces.
	if strings.HasPrefix(s, "{") {
		if strings.HasSuffix(s, "}") && hasBalancedInspectContainer(s) {
			return parseInspectMap(s[1 : len(s)-1])
		}

		// A broken top-level object should fail.
		return nil, false
	}

	// Array-like value only when the WHOLE value is wrapped by balanced brackets.
	// Important: "[script] null" is NOT an array. It is a scalar string.
	if strings.HasPrefix(s, "[") {
		if strings.HasSuffix(s, "]") && hasBalancedInspectContainer(s) {
			return parseInspectArray(s[1 : len(s)-1])
		}

		return parseInspectScalar(s), true
	}

	return parseInspectScalar(s), true
}

func parseInspectMap(inner string) (map[string]any, bool) {
	out := map[string]any{}

	inner = strings.TrimSpace(inner)
	if inner == "" {
		return out, true
	}

	parts := splitInspectTopLevel(inner, ',')
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}

		k, v, ok := cutInspectKeyValue(part)
		if !ok {
			return nil, false
		}

		key := strings.TrimSpace(k)
		key = strings.Trim(key, `"'`)

		if key == "" {
			return nil, false
		}

		parsed, ok := parseInspectValue(v)
		if !ok {
			// Do not fail the whole object because one value is not a valid
			// container. Keep it as a scalar string.
			parsed = strings.TrimSpace(v)
		}

		out[key] = parsed
	}

	return out, true
}

func parseInspectArray(inner string) ([]any, bool) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return []any{}, true
	}

	parts := splitInspectTopLevel(inner, ',')
	out := make([]any, 0, len(parts))

	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}

		v, ok := parseInspectValue(part)
		if !ok {
			return nil, false
		}

		out = append(out, v)
	}

	return out, true
}

func cutInspectKeyValue(s string) (string, string, bool) {
	depth := 0
	quote := rune(0)
	escaped := false

	for i, r := range s {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}

			if r == '\\' {
				escaped = true
				continue
			}

			if r == quote {
				quote = 0
			}

			continue
		}

		switch r {
		case '\'', '"':
			quote = r

		case '{', '[':
			depth++

		case '}', ']':
			if depth > 0 {
				depth--
			}

		case ':':
			if depth == 0 {
				return s[:i], s[i+1:], true
			}
		}
	}

	return "", "", false
}

func splitInspectTopLevel(s string, sep rune) []string {
	var out []string

	start := 0
	depth := 0
	quote := rune(0)
	escaped := false

	for i, r := range s {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}

			if r == '\\' {
				escaped = true
				continue
			}

			if r == quote {
				quote = 0
			}

			continue
		}

		switch r {
		case '\'', '"':
			quote = r

		case '{':
			depth++

		case '[':
			// Only treat [ as nested structure when it appears to be a real
			// array token, not plain text like "[script] null".
			if looksLikeInspectArrayAt(s, i) {
				depth++
			}

		case '}':
			if depth > 0 {
				depth--
			}

		case ']':
			if depth > 0 {
				depth--
			}

		default:
			if r == sep && depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + len(string(r))
			}
		}
	}

	out = append(out, strings.TrimSpace(s[start:]))
	return out
}

func looksLikeInspectArrayAt(s string, idx int) bool {
	if idx < 0 || idx >= len(s) || s[idx] != '[' {
		return false
	}

	// Find matching closing bracket.
	depth := 0
	quote := byte(0)
	escaped := false

	for i := idx; i < len(s); i++ {
		c := s[i]

		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}

		switch c {
		case '\'', '"':
			quote = c

		case '[':
			depth++

		case ']':
			depth--
			if depth == 0 {
				rest := strings.TrimSpace(s[i+1:])

				// Valid array token when it ends here or is followed by a map/array
				// separator. If followed by normal text, it is scalar text.
				return rest == "" || strings.HasPrefix(rest, ",") || strings.HasPrefix(rest, "}") || strings.HasPrefix(rest, "]")
			}
		}
	}

	return false
}

func parseInspectScalar(s string) any {
	s = strings.TrimSpace(s)

	switch s {
	case "":
		return ""
	case "null", "<nil>", "nil":
		return nil
	case "true":
		return true
	case "false":
		return false
	}

	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			if unquoted, err := strconv.Unquote(s); err == nil {
				return unquoted
			}
			return s[1 : len(s)-1]
		}

		if s[0] == '\'' && s[len(s)-1] == '\'' {
			return unescapeSingleQuotedInspectString(s[1 : len(s)-1])
		}
	}

	if looksInteger(s) {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		}

		if u, err := strconv.ParseUint(s, 10, 64); err == nil {
			return u
		}
	}

	if looksFloat(s) {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return s
			}
			return f
		}
	}

	return s
}

func looksInteger(s string) bool {
	if s == "" {
		return false
	}

	i := 0
	if s[0] == '-' || s[0] == '+' {
		if len(s) == 1 {
			return false
		}
		i = 1
	}

	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}

	return true
}

func looksFloat(s string) bool {
	if s == "" {
		return false
	}

	hasDigit := false
	hasFloatMarker := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		switch {
		case c >= '0' && c <= '9':
			hasDigit = true

		case c == '.' || c == 'e' || c == 'E':
			hasFloatMarker = true

		case c == '-' || c == '+':
			if i == 0 {
				continue
			}

			prev := s[i-1]
			if prev == 'e' || prev == 'E' {
				continue
			}

			return false

		default:
			return false
		}
	}

	return hasDigit && hasFloatMarker
}

func hasBalancedInspectContainer(s string) bool {
	if len(s) < 2 {
		return false
	}

	first := s[0]
	last := s[len(s)-1]

	switch first {
	case '{':
		if last != '}' {
			return false
		}
	case '[':
		if last != ']' {
			return false
		}
	default:
		return false
	}

	stack := make([]rune, 0, 8)
	quote := rune(0)
	escaped := false

	for _, r := range s {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}

			if r == '\\' {
				escaped = true
				continue
			}

			if r == quote {
				quote = 0
			}

			continue
		}

		switch r {
		case '\'', '"':
			quote = r

		case '{':
			stack = append(stack, '}')

		case '[':
			stack = append(stack, ']')

		case '}', ']':
			if len(stack) == 0 {
				return false
			}

			expected := stack[len(stack)-1]
			if r != expected {
				return false
			}

			stack = stack[:len(stack)-1]
		}
	}

	return len(stack) == 0 && quote == 0 && !escaped
}

func unescapeSingleQuotedInspectString(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	escaped := false
	for _, r := range s {
		if escaped {
			switch r {
			case '\\', '\'':
				b.WriteRune(r)
			case 'n':
				b.WriteRune('\n')
			case 'r':
				b.WriteRune('\r')
			case 't':
				b.WriteRune('\t')
			default:
				b.WriteRune('\\')
				b.WriteRune(r)
			}
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			continue
		}

		b.WriteRune(r)
	}

	if escaped {
		b.WriteRune('\\')
	}

	return b.String()
}
