package runtime

import (
	"net/http"
	"strings"
)

type RouteHandler struct {
	Method  string
	Pattern string // e.g. /users/:id
	Segments []routeSegment
	Handler func(w http.ResponseWriter, r *http.Request, params map[string]string)
}

type routeSegment struct {
	Literal string
	Param   string // non-empty means it's a :param
}

type Router struct {
	routes []RouteHandler
}

func NewRouter() *Router {
	return &Router{}
}

func (rt *Router) Add(method, pattern string, handler func(http.ResponseWriter, *http.Request, map[string]string)) {
	segs := parsePattern(pattern)
	rt.routes = append(rt.routes, RouteHandler{
		Method:   method,
		Pattern:  pattern,
		Segments: segs,
		Handler:  handler,
	})
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, route := range rt.routes {
		if route.Method != r.Method {
			continue
		}
		params, ok := matchPath(route.Segments, r.URL.Path)
		if ok {
			route.Handler(w, r, params)
			return
		}
	}
	w.WriteHeader(404)
	w.Write([]byte(`{"error":"not found"}`))
}

func parsePattern(pattern string) []routeSegment {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	segs := make([]routeSegment, len(parts))
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			segs[i] = routeSegment{Param: p[1:]}
		} else {
			segs[i] = routeSegment{Literal: p}
		}
	}
	return segs
}

func matchPath(segments []routeSegment, path string) (map[string]string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != len(segments) {
		return nil, false
	}
	params := make(map[string]string)
	for i, seg := range segments {
		if seg.Param != "" {
			params[seg.Param] = parts[i]
		} else if seg.Literal != parts[i] {
			return nil, false
		}
	}
	return params, true
}
