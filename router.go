// Copyright 2016 Qiang Xue. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package routing provides high performance and powerful HTTP routing capabilities.
package neo

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	radix "github.com/armon/go-radix"
)

type (
	// Handler is the function for handling HTTP requests.
	Handler func(*Context) error

	// Router manages routes and dispatches HTTP requests to the handlers of the matching routes.
	Router struct {
		RouteGroup
		IgnoreTrailingSlash bool // whether to ignore trailing slashes in the end of the request URL
		UseEscapedPath      bool // whether to use encoded URL instead of decoded URL to match routes
		pool                sync.Pool
		routes              []*Route
		namedRoutes         map[string]*Route
		allowStore          *store
		stores              map[string]routeStore
		maxParams           int
		notFound            []Handler
		notFoundHandlers    []Handler

		catchAll    *radix.Tree
		IPExtractor IPExtractor
	}

	// routeStore stores route paths and the corresponding handlers.
	routeStore interface {
		Add(key string, data interface{}) int
		Get(key string, pvalues []string) (data interface{}, pnames []string)
		String() string
	}
)

// Methods lists all supported HTTP methods by Router.
var Methods = []string{
	"CONNECT",
	"DELETE",
	"GET",
	"HEAD",
	"OPTIONS",
	"PATCH",
	"POST",
	"PUT",
	"TRACE",
}

const (
	methodBitConnect uint16 = 1 << iota
	methodBitDelete
	methodBitGet
	methodBitHead
	methodBitOptions
	methodBitPatch
	methodBitPost
	methodBitPut
	methodBitTrace
	allowHeaderTableSize = 1 << 9
)

var allowHeaders = buildAllowHeaders()

func buildAllowHeaders() [allowHeaderTableSize]string {
	var headers [allowHeaderTableSize]string
	for mask := 0; mask < len(headers); mask++ {
		headers[mask] = buildAllowHeader(uint16(mask))
	}
	return headers
}

func buildAllowHeader(bits uint16) string {
	if bits == 0 {
		return ""
	}
	var b strings.Builder
	for _, method := range Methods {
		if bits&methodBit(method) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		b.WriteString(method)
	}
	return b.String()
}

func methodBit(method string) uint16 {
	switch method {
	case "CONNECT":
		return methodBitConnect
	case "DELETE":
		return methodBitDelete
	case "GET":
		return methodBitGet
	case "HEAD":
		return methodBitHead
	case "OPTIONS":
		return methodBitOptions
	case "PATCH":
		return methodBitPatch
	case "POST":
		return methodBitPost
	case "PUT":
		return methodBitPut
	case "TRACE":
		return methodBitTrace
	default:
		return 0
	}
}

func mergeMethodBits(existing, data interface{}) interface{} {
	return existing.(uint16) | data.(uint16)
}

// New creates a new Router object.
func New() *Router {
	r := &Router{
		namedRoutes: make(map[string]*Route),
		allowStore:  newStore(),
		stores:      make(map[string]routeStore),
		catchAll:    radix.New(),
	}
	r.RouteGroup = *newRouteGroup("", r, make([]Handler, 0))
	r.NotFound(MethodNotAllowedHandler, NotFoundHandler)
	r.pool.New = func() interface{} {
		return &Context{
			pvalues: make([]string, r.maxParams),
			router:  r,
		}
	}
	return r
}

// ServeHTTP handles the HTTP request.
// It is required by http.Handler
func (r *Router) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	c := r.pool.Get().(*Context)
	if cap(c.pvalues) < r.maxParams {
		c.pvalues = make([]string, r.maxParams)
	} else {
		c.pvalues = c.pvalues[:r.maxParams]
	}
	c.init(res, req)

	path := req.URL.Path
	if r.UseEscapedPath {
		path = req.URL.EscapedPath()
	}
	path = r.normalizeRequestPath(path)

	c.handlers, c.pnames, c.allow = r.find(req.Method, path, c.pvalues)
	if r.UseEscapedPath {
		for i := 0; i < len(c.pnames); i++ {
			v := c.pvalues[i]
			if strings.IndexByte(v, '%') >= 0 || strings.IndexByte(v, '+') >= 0 {
				c.pvalues[i], _ = url.QueryUnescape(v)
			}
		}
	}
	if err := c.Next(); err != nil {
		r.handleError(c, err)
	}
	r.pool.Put(c)
}

// Route returns the named route.
// Nil is returned if the named route cannot be found.
func (r *Router) Route(name string) *Route {
	return r.namedRoutes[name]
}

// Routes returns all routes managed by the router.
func (r *Router) Routes() []*Route {
	return r.routes
}

// Use appends the specified handlers to the router and shares them with all routes.
func (r *Router) Use(handlers ...Handler) {
	r.RouteGroup.Use(handlers...)
	r.notFoundHandlers = combineHandlers(r.handlers, r.notFound)
}

// NotFound specifies the handlers that should be invoked when the router cannot find any route matching a request.
// Note that the handlers registered via Use will be invoked first in this case.
func (r *Router) NotFound(handlers ...Handler) {
	r.notFound = handlers
	r.notFoundHandlers = combineHandlers(r.handlers, r.notFound)
}

// Find determines the handlers and parameters to use for a specified method and path.
func (r *Router) Find(method, path string) (handlers []Handler, params map[string]string) {
	pvalues := make([]string, r.maxParams)
	handlers, pnames, _ := r.find(method, path, pvalues)
	params = make(map[string]string, len(pnames))
	for i, n := range pnames {
		params[n] = pvalues[i]
	}
	return handlers, params
}

// handleError is the error handler for handling any unhandled errors.
func (r *Router) handleError(c *Context, err error) {
	if httpError, ok := err.(HTTPError); ok {
		http.Error(c.Response, httpError.Error(), httpError.StatusCode())
	} else {
		http.Error(c.Response, err.Error(), http.StatusInternalServerError)
	}
}

func (r *Router) addRoute(route *Route, handlers []Handler) {
	path := route.group.prefix + route.path

	r.routes = append(r.routes, route)

	store := r.stores[route.method]
	if store == nil {
		store = newStore()
		r.stores[route.method] = store
	}

	// an asterisk at the end matches any number of characters
	if strings.HasSuffix(path, "*") {
		path = path[:len(path)-1] + "<:.*>"
	}

	if n := store.Add(path, handlers); n > r.maxParams {
		r.maxParams = n
	}
	if n := r.allowStore.AddOrMerge(path, methodBit(route.method), mergeMethodBits); n > r.maxParams {
		r.maxParams = n
	}
}

func (r *Router) find(method, path string, pvalues []string) (handlers []Handler, pnames []string, allow string) {
	var hh interface{}
	if store := r.stores[method]; store != nil {
		hh, pnames = store.Get(path, pvalues)
	}
	if hh != nil {
		return hh.([]Handler), pnames, ""
	}

	_, hh, ok := r.catchAll.LongestPrefix(path)
	if ok {
		return hh.([]Handler), pnames, ""
	}

	return r.notFoundHandlers, pnames, r.findAllowedHeader(path)
}

func (r *Router) findAllowedMethodBits(path string) uint16 {
	var bits uint16
	r.allowStore.MatchAll(path, func(data interface{}) {
		bits |= data.(uint16)
	})
	return bits
}

func (r *Router) findAllowedHeader(path string) string {
	bits := r.findAllowedMethodBits(path)
	if bits == 0 {
		return ""
	}
	return allowHeaders[int(bits|methodBitOptions)]
}

func (r *Router) findAllowedMethods(path string) map[string]bool {
	bits := r.findAllowedMethodBits(path)
	if bits == 0 {
		return nil
	}

	methods := make(map[string]bool, len(Methods))
	for _, method := range Methods {
		if bits&methodBit(method) != 0 {
			methods[method] = true
		}
	}
	return methods
}

func (r *Router) FindAllowedMethods(path string) map[string]bool {
	return r.findAllowedMethods(path)
}

func (r *Router) normalizeRequestPath(path string) string {
	if !r.IgnoreTrailingSlash || len(path) <= 1 || path[len(path)-1] != '/' {
		return path
	}
	i := len(path) - 1
	for i > 0 && path[i] == '/' {
		i--
	}
	if i == 0 {
		return path[:1]
	}
	return path[:i+1]
}

// NotFoundHandler returns a 404 HTTP error indicating a request has no matching route.
func NotFoundHandler(*Context) error {
	return defaultNotFoundHTTPError
}

// MethodNotAllowedHandler handles the situation when a request has matching route without matching HTTP method.
// In this case, the handler will respond with an Allow HTTP header listing the allowed HTTP methods.
// Otherwise, the handler will do nothing and let the next handler (usually a NotFoundHandler) to handle the problem.
func MethodNotAllowedHandler(c *Context) error {
	allow := c.allow
	if allow == "" && c.Router() != nil && c.Request != nil {
		path := c.Request.URL.Path
		if c.Router().UseEscapedPath {
			path = c.Request.URL.EscapedPath()
		}
		allow = c.Router().findAllowedHeader(c.Router().normalizeRequestPath(path))
	}
	if allow == "" {
		return nil
	}
	c.Response.Header().Set(HeaderAllow, allow)
	if c.Request.Method != "OPTIONS" {
		c.Response.WriteHeader(http.StatusMethodNotAllowed)
	}
	c.Abort()
	return nil
}

// HTTPHandlerFunc adapts a http.HandlerFunc into a mat.Handler.
func HTTPHandlerFunc(h http.HandlerFunc) Handler {
	return func(c *Context) error {
		h(c.Response, c.Request)
		return nil
	}
}

// HTTPHandler adapts a http.Handler into a mat.Handler.
func HTTPHandler(h http.Handler) Handler {
	return func(c *Context) error {
		h.ServeHTTP(c.Response, c.Request)
		return nil
	}
}
