// Package localhttp exposes the process-local HTTP boundary for Mind Weaver.
//
// It deliberately provides only exact, pre-registered routes. It has no proxy,
// wildcard, redirect, CORS, account, or bearer-token behavior.
//
// Start returns the bootstrap secret to a trusted launcher, but this package
// does not yet provide an unauthenticated HTML bootstrap shell or a WebView
// integration capable of delivering the resulting HttpOnly cookie. Therefore
// it is a hardened transport primitive, not yet a complete browser launch flow.
package localhttp

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
)

var (
	// ErrInvalidRoute means a route is not an exact /api/ path or uses an
	// unsupported HTTP method.
	ErrInvalidRoute = errors.New("invalid local HTTP route")
	// ErrDuplicateRoute means the same method and path were registered twice.
	ErrDuplicateRoute = errors.New("duplicate local HTTP route")
	// ErrRouterSealed means registration was attempted after Start sealed the
	// router. Runtime route mutation is intentionally unsupported.
	ErrRouterSealed = errors.New("local HTTP router is sealed")
)

type routeKey struct {
	method string
	path   string
}

// Router stores a fixed set of exact business endpoints. Registration is only
// allowed before Start. Health and bootstrap endpoints are owned by Server and
// cannot be replaced through Router.
type Router struct {
	mu     sync.RWMutex
	routes map[routeKey]http.Handler
	sealed bool
}

// NewRouter returns an empty router.
func NewRouter() *Router {
	return &Router{routes: make(map[routeKey]http.Handler)}
}

// Handle registers one exact business endpoint. path must be a clean ASCII
// path below /api/ and cannot contain variables, wildcards, query strings, or a
// trailing slash. Only the small method set used by the local API is accepted.
func (r *Router) Handle(method, routePath string, handler http.Handler) error {
	if r == nil || handler == nil || !validMethod(method) || !validRoutePath(routePath) {
		return ErrInvalidRoute
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return ErrRouterSealed
	}
	key := routeKey{method: method, path: routePath}
	if _, exists := r.routes[key]; exists {
		return ErrDuplicateRoute
	}
	r.routes[key] = handler
	return nil
}

// HandleFunc is Handle for an http.HandlerFunc.
func (r *Router) HandleFunc(method, routePath string, handler http.HandlerFunc) error {
	if handler == nil {
		return ErrInvalidRoute
	}
	return r.Handle(method, routePath, handler)
}

func (r *Router) seal() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sealed = true
}

func (r *Router) serveBusiness(response http.ResponseWriter, request *http.Request, maxResponseBytes int64) {
	r.mu.RLock()
	handler := r.routes[routeKey{method: request.Method, path: request.URL.Path}]
	pathExists := false
	if handler == nil {
		for key := range r.routes {
			if key.path == request.URL.Path {
				pathExists = true
				break
			}
		}
	}
	r.mu.RUnlock()

	if handler == nil {
		if pathExists {
			writeSafeError(response, http.StatusMethodNotAllowed)
			return
		}
		writeSafeError(response, http.StatusNotFound)
		return
	}
	buffered := newSafeBusinessResponse(maxResponseBytes)
	handler.ServeHTTP(buffered, request)
	buffered.flushTo(response)
}

func validMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func validRoutePath(routePath string) bool {
	if !strings.HasPrefix(routePath, "/api/") || strings.HasSuffix(routePath, "/") || path.Clean(routePath) != routePath {
		return false
	}
	for _, character := range routePath {
		if character < 0x21 || character > 0x7e || character == '?' || character == '#' || character == '%' || character == '*' || character == '{' || character == '}' {
			return false
		}
	}
	return true
}

// safeBusinessResponse prevents an endpoint implementation from returning an
// error body or error headers containing internal details. It also keeps
// successful output bounded and unobservable until the handler returns. A
// panic or timeout therefore cannot expose a partial successful response.
type safeBusinessResponse struct {
	header      http.Header
	body        bytes.Buffer
	maxBytes    int64
	status      int
	wroteHeader bool
	errorStatus int
	overflow    bool
}

func newSafeBusinessResponse(maxBytes int64) *safeBusinessResponse {
	return &safeBusinessResponse{header: make(http.Header), maxBytes: maxBytes}
}

func (response *safeBusinessResponse) Header() http.Header { return response.header }

func (response *safeBusinessResponse) WriteHeader(status int) {
	if response.wroteHeader {
		return
	}
	response.wroteHeader = true
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		response.errorStatus = safeStatus(status)
		return
	}
	response.status = status
}

func (response *safeBusinessResponse) Write(body []byte) (int, error) {
	if !response.wroteHeader {
		response.WriteHeader(http.StatusOK)
	}
	if response.errorStatus != 0 || response.overflow {
		return len(body), nil
	}
	if int64(len(body)) > response.maxBytes-int64(response.body.Len()) {
		response.body.Reset()
		response.header = make(http.Header)
		response.overflow = true
		return 0, errors.New("local HTTP response exceeds configured limit")
	}
	return response.body.Write(body)
}

func (response *safeBusinessResponse) flushTo(destination http.ResponseWriter) {
	if response.overflow {
		writeSafeError(destination, http.StatusInternalServerError)
		return
	}
	if response.errorStatus != 0 {
		writeSafeError(destination, response.errorStatus)
		return
	}
	for name, values := range response.header {
		for _, value := range values {
			destination.Header().Add(name, value)
		}
	}
	setSecurityHeaders(destination.Header())
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	destination.WriteHeader(status)
	_, _ = io.Copy(destination, &response.body)
}

func safeStatus(status int) int {
	if status < 300 || status > 599 {
		return http.StatusInternalServerError
	}
	return status
}
