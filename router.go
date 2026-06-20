package fasthttp

import (
	"strings"
	"unsafe"
)

type HandlerFunc func(*Ctx) error

type Param struct {
	Key   string
	Value string
}

type node struct {
	path     string
	children []*node
	handler  HandlerFunc
	isParam  bool
	isWild   bool
	paramKey string
}

type Router struct {
	trees       map[string]*node
	staticCache map[string]HandlerFunc
}

func newRouter() *Router {
	return &Router{
		trees:       make(map[string]*node, 8),
		staticCache: make(map[string]HandlerFunc, 32),
	}
}

func (r *Router) Add(method, path string, h HandlerFunc) {
	if r.trees[method] == nil {
		r.trees[method] = &node{}
	}
	insert(r.trees[method], path, h)
	// Cache static routes (no params, no wildcards)
	if !hasParams(path) {
		key := method + ":" + path
		r.staticCache[key] = h
	}
}

func hasParams(path string) bool {
	for i := 0; i < len(path); i++ {
		if path[i] == ':' || path[i] == '*' {
			return true
		}
	}
	return false
}

func (r *Router) FindBytes(method, path []byte, params *[]Param) HandlerFunc {
	// Fast path: check static cache first
	if h := r.staticCache[string(method)+":"+string(path)]; h != nil {
		return h
	}
	// Fall back to radix tree
	root := r.trees[string(method)]
	if root == nil {
		return nil
	}
	return match(root, path, params)
}

func (r *Router) Find(method, path string, params *[]Param) HandlerFunc {
	root := r.trees[method]
	if root == nil {
		return nil
	}
	return match(root, []byte(path), params)
}

func insert(n *node, path string, h HandlerFunc) {
	if path == "" || path == "/" {
		n.handler = h
		return
	}

	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	seg, rest := splitPath(path)

	for _, child := range n.children {
		if child.path == seg || (child.isParam && strings.HasPrefix(seg, ":")) ||
			(child.isWild && seg == "*") {
			if rest == "" {
				child.handler = h
			} else {
				insert(child, rest, h)
			}
			return
		}
	}

	child := &node{path: seg}
	if strings.HasPrefix(seg, ":") {
		child.isParam = true
		child.paramKey = seg[1:]
	} else if seg == "*" {
		child.isWild = true
	}
	n.children = append(n.children, child)

	if rest == "" {
		child.handler = h
	} else {
		insert(child, rest, h)
	}
}

func match(n *node, path []byte, params *[]Param) HandlerFunc {
	if len(path) == 0 || (len(path) == 1 && path[0] == '/') {
		return n.handler
	}
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	seg, rest := splitPathBytes(path)

	for _, child := range n.children {
		if child.isWild {
			*params = append(*params, Param{Key: "*", Value: b2s(path)})
			return child.handler
		}
		if child.isParam {
			*params = append(*params, Param{Key: child.paramKey, Value: b2s(seg)})
			if len(rest) == 0 {
				return child.handler
			}
			return match(child, rest, params)
		}
		if strEqBytes(child.path, seg) {
			if len(rest) == 0 {
				return child.handler
			}
			return match(child, rest, params)
		}
	}
	return nil
}

func splitPath(path string) (seg, rest string) {
	i := strings.IndexByte(path, '/')
	if i < 0 {
		return path, ""
	}
	return path[:i], path[i:]
}

func splitPathBytes(path []byte) (seg, rest []byte) {
	for i, c := range path {
		if c == '/' {
			return path[:i], path[i:]
		}
	}
	return path, nil
}

// b2s converts []byte to string without allocation.
func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// strEqBytes reports whether a string and a byte slice are equal — zero alloc.
func strEqBytes(s string, b []byte) bool {
	if len(s) != len(b) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != b[i] {
			return false
		}
	}
	return true
}
