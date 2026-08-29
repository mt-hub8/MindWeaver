package localhttp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	HealthPath            = "/healthz"
	BootstrapExchangePath = "/bootstrap/exchange"
	CSRFRefreshPath       = "/session/csrf"
	BootstrapHeader       = "X-MindWeaver-Bootstrap"
	CSRFHeader            = "X-MindWeaver-CSRF"
	NonBrowserHeader      = "X-MindWeaver-Client"
	NonBrowserReadOnly    = "local-readonly"

	defaultMaxBodyBytes = int64(1 << 20)
	defaultMaxResponse  = int64(4 << 20)
	maximumBodyBytes    = int64(64 << 20)
	maximumResponse     = int64(64 << 20)
	tokenBytes          = 32
	cookieNameBytes     = 16
	sessionCookiePrefix = "mindweaver_session_"
	defaultBootstrapTTL = 2 * time.Minute
	maximumBootstrapTTL = 10 * time.Minute
)

var errRandomness = errors.New("secure randomness unavailable")

// Config contains bounded server controls. Zero values select conservative
// defaults. Values below zero are rejected.
type Config struct {
	MaxBodyBytes      int64
	MaxResponseBytes  int64
	RequestTimeout    time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	BootstrapTTL      time.Duration
}

// BootstrapToken is the only bootstrap credential copy returned by Start.
// String intentionally redacts it. HeaderValue should be kept only long enough
// to make the first exchange and must never be logged or placed in a URL.
type BootstrapToken struct {
	value [tokenBytes]byte
}

func (token BootstrapToken) String() string { return "[REDACTED]" }

// GoString also redacts %#v formatting, which otherwise bypasses String for
// struct values and could expose the private byte array in diagnostics.
func (token BootstrapToken) GoString() string { return "localhttp.BootstrapToken([REDACTED])" }

// HeaderValue returns the token encoding accepted in BootstrapHeader.
func (token BootstrapToken) HeaderValue() string {
	return base64.RawURLEncoding.EncodeToString(token.value[:])
}

type sessionState struct {
	cookieHash [sha256.Size]byte
	csrfHash   [sha256.Size]byte
	csrfToken  [tokenBytes]byte
}

// Server owns one loopback listener and one process-memory browser session.
// It never logs requests or credentials and has no persistent authentication
// state. A hostile process running as the same OS account remains outside this
// boundary and should be addressed by OS account and Vault permissions.
type Server struct {
	host   string
	origin string

	listener net.Listener
	http     *http.Server
	done     chan error

	mu             sync.Mutex
	bootstrapHash  [sha256.Size]byte
	bootstrapLive  bool
	bootstrapUntil time.Time
	session        *sessionState
	sessionCookie  string
	closed         bool

	maxBodyBytes int64
	maxResponse  int64
	router       *Router
}

// Start binds a random IPv4 loopback port, seals router, and starts serving.
// The returned bootstrap token is high-entropy and accepted exactly once.
func Start(router *Router, config Config) (*Server, BootstrapToken, error) {
	config, err := normalizeConfig(config)
	if err != nil {
		return nil, BootstrapToken{}, err
	}
	if router == nil {
		router = NewRouter()
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, BootstrapToken{}, fmt.Errorf("listen on loopback: %w", err)
	}
	closeListener := true
	defer func() {
		if closeListener {
			_ = listener.Close()
		}
	}()

	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok || tcpAddress.IP.String() != "127.0.0.1" || tcpAddress.Port == 0 {
		return nil, BootstrapToken{}, errors.New("listener did not bind exact IPv4 loopback")
	}

	var bootstrap BootstrapToken
	if _, err := io.ReadFull(rand.Reader, bootstrap.value[:]); err != nil {
		return nil, BootstrapToken{}, fmt.Errorf("%w: %v", errRandomness, err)
	}
	var cookieNameEntropy [cookieNameBytes]byte
	if _, err := io.ReadFull(rand.Reader, cookieNameEntropy[:]); err != nil {
		return nil, BootstrapToken{}, fmt.Errorf("%w: %v", errRandomness, err)
	}
	encodedBootstrap := bootstrap.HeaderValue()
	host := listener.Addr().String()
	server := &Server{
		host:           host,
		origin:         "http://" + host,
		listener:       listener,
		done:           make(chan error, 1),
		bootstrapHash:  sha256.Sum256([]byte(encodedBootstrap)),
		bootstrapLive:  true,
		bootstrapUntil: time.Now().Add(config.BootstrapTTL),
		sessionCookie:  sessionCookiePrefix + hex.EncodeToString(cookieNameEntropy[:]),
		maxBodyBytes:   config.MaxBodyBytes,
		maxResponse:    config.MaxResponseBytes,
		router:         router,
	}
	router.seal()

	requestHandler := http.HandlerFunc(server.dispatch)
	// TimeoutHandler executes its wrapped handler in a child goroutine. Recovery
	// must therefore be inside that boundary; placing recoverSafely outside it
	// would let a business-handler panic terminate the whole process.
	handler := securityHeaders(http.TimeoutHandler(recoverSafely(requestHandler), config.RequestTimeout, "request rejected\n"))
	server.http = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
		MaxHeaderBytes:    32 << 10,
		ErrorLog:          log.New(io.Discard, "", 0),
	}

	closeListener = false
	go func() {
		err := server.http.Serve(listener)
		server.mu.Lock()
		closed := server.closed
		server.mu.Unlock()
		if errors.Is(err, http.ErrServerClosed) || (closed && errors.Is(err, net.ErrClosed)) {
			err = nil
		}
		server.done <- err
		close(server.done)
	}()
	return server, bootstrap, nil
}

// Origin is the only browser origin accepted by the server.
func (server *Server) Origin() string {
	if server == nil {
		return ""
	}
	return server.origin
}

// Addr returns the bound 127.0.0.1 listener address.
func (server *Server) Addr() net.Addr {
	if server == nil {
		return nil
	}
	return server.listener.Addr()
}

// Done is closed after serving stops and yields at most one serve error.
func (server *Server) Done() <-chan error {
	if server == nil {
		closed := make(chan error)
		close(closed)
		return closed
	}
	return server.done
}

// Quiesce synchronously closes the listener and invalidates process-memory
// credentials without waiting for active handlers. Shutdown can then drain
// those handlers under the App's single process-wide shutdown deadline.
func (server *Server) Quiesce() error {
	if server == nil {
		return nil
	}
	server.mu.Lock()
	alreadyClosed := server.closed
	server.closed = true
	server.bootstrapLive = false
	server.bootstrapHash = [sha256.Size]byte{}
	server.bootstrapUntil = time.Time{}
	server.session = nil
	server.mu.Unlock()
	if alreadyClosed {
		return nil
	}
	if err := server.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("quiesce local HTTP server: %w", err)
	}
	return nil
}

// Shutdown invalidates process-memory credentials before gracefully stopping
// the HTTP server. It is safe to call more than once.
func (server *Server) Shutdown(ctx context.Context) error {
	if server == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("shutdown context is required")
	}
	if err := server.Quiesce(); err != nil {
		return err
	}
	if err := server.http.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("shutdown local HTTP server: %w", err)
	}
	return nil
}

// Close force-closes active HTTP connections after invalidating all in-memory
// credentials. App uses this only when graceful shutdown exceeds its bound.
func (server *Server) Close() error {
	if server == nil {
		return nil
	}
	server.mu.Lock()
	server.closed = true
	server.bootstrapLive = false
	server.bootstrapHash = [sha256.Size]byte{}
	server.bootstrapUntil = time.Time{}
	server.session = nil
	server.mu.Unlock()
	if err := server.http.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("close local HTTP server: %w", err)
	}
	return nil
}

func (server *Server) dispatch(response http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(response, request.Body, server.maxBodyBytes)
	if request.ContentLength > server.maxBodyBytes {
		writeSafeError(response, http.StatusRequestEntityTooLarge)
		return
	}
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) && (request.ContentLength > 0 || len(request.TransferEncoding) > 0) {
		writeSafeError(response, http.StatusBadRequest)
		return
	}
	public := server.router.isPublic(request.Method, request.URL.Path)
	if !server.validTransportRequest(request, public) {
		writeSafeError(response, http.StatusForbidden)
		return
	}

	switch request.URL.Path {
	case HealthPath:
		server.serveHealth(response, request)
		return
	case BootstrapExchangePath:
		server.serveBootstrapExchange(response, request)
		return
	}
	if public {
		server.router.servePublic(response, request, server.maxResponse)
		return
	}

	if !server.authenticated(request) {
		writeSafeError(response, http.StatusUnauthorized)
		return
	}
	if request.URL.Path == CSRFRefreshPath {
		server.serveCSRFRefresh(response, request)
		return
	}
	if needsCSRF(request.Method) && !server.validCSRF(request) {
		writeSafeError(response, http.StatusForbidden)
		return
	}
	server.router.serveBusiness(response, request, server.maxResponse)
}

func (server *Server) validTransportRequest(request *http.Request, public bool) bool {
	if request.Host != server.host || request.URL.IsAbs() || request.URL.Host != "" || request.URL.RawPath != "" || request.URL.Fragment != "" {
		return false
	}
	if request.URL.EscapedPath() != request.URL.Path {
		return false
	}
	remoteHost, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil || remoteHost != "127.0.0.1" {
		return false
	}

	origins := request.Header.Values("Origin")
	if len(origins) == 1 {
		return origins[0] == server.origin
	}
	if len(origins) != 0 || (request.Method != http.MethodGet && request.Method != http.MethodHead) {
		return false
	}
	if validTopLevelNavigation(request.Header) {
		return true
	}
	if sameOriginBrowserRead(request.Header, public) {
		return true
	}
	clients := request.Header.Values(NonBrowserHeader)
	return len(clients) == 1 && clients[0] == NonBrowserReadOnly && !hasBrowserFetchHeaders(request.Header)
}

func sameOriginBrowserRead(header http.Header, public bool) bool {
	sites := header.Values("Sec-Fetch-Site")
	modes := header.Values("Sec-Fetch-Mode")
	destinations := header.Values("Sec-Fetch-Dest")
	if len(sites) != 1 || len(modes) != 1 || len(destinations) != 1 || sites[0] != "same-origin" {
		return false
	}
	if public {
		return modes[0] == "no-cors" && (destinations[0] == "script" || destinations[0] == "style")
	}
	return modes[0] == "cors" && destinations[0] == "empty"
}

func validTopLevelNavigation(header http.Header) bool {
	sites := header.Values("Sec-Fetch-Site")
	modes := header.Values("Sec-Fetch-Mode")
	destinations := header.Values("Sec-Fetch-Dest")
	if len(sites) != 1 || len(modes) != 1 || len(destinations) != 1 {
		return false
	}
	return (sites[0] == "same-origin" || sites[0] == "none") && modes[0] == "navigate" && destinations[0] == "document"
}

func hasBrowserFetchHeaders(header http.Header) bool {
	for name := range header {
		if strings.HasPrefix(http.CanonicalHeaderKey(name), "Sec-Fetch-") {
			return true
		}
	}
	return false
}

func (server *Server) serveHealth(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writeSafeError(response, http.StatusMethodNotAllowed)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if request.Method == http.MethodGet {
		_, _ = io.WriteString(response, "{\"status\":\"ok\"}\n")
	}
}

func (server *Server) serveBootstrapExchange(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeSafeError(response, http.StatusMethodNotAllowed)
		return
	}
	if request.ContentLength > 0 || len(request.TransferEncoding) > 0 {
		writeSafeError(response, http.StatusBadRequest)
		return
	}
	values := request.Header.Values(BootstrapHeader)
	if len(values) != 1 || len(values[0]) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		writeSafeError(response, http.StatusUnauthorized)
		return
	}

	var sessionBytes [tokenBytes]byte
	var csrfBytes [tokenBytes]byte
	if _, err := io.ReadFull(rand.Reader, sessionBytes[:]); err != nil {
		writeSafeError(response, http.StatusServiceUnavailable)
		return
	}
	if _, err := io.ReadFull(rand.Reader, csrfBytes[:]); err != nil {
		writeSafeError(response, http.StatusServiceUnavailable)
		return
	}
	sessionValue := base64.RawURLEncoding.EncodeToString(sessionBytes[:])
	csrfValue := base64.RawURLEncoding.EncodeToString(csrfBytes[:])
	presentedHash := sha256.Sum256([]byte(values[0]))

	server.mu.Lock()
	now := time.Now()
	if server.bootstrapLive && !now.Before(server.bootstrapUntil) {
		server.bootstrapLive = false
		server.bootstrapHash = [sha256.Size]byte{}
		server.bootstrapUntil = time.Time{}
	}
	valid := !server.closed && server.bootstrapLive && subtle.ConstantTimeCompare(presentedHash[:], server.bootstrapHash[:]) == 1
	if valid {
		server.bootstrapLive = false
		server.bootstrapHash = [sha256.Size]byte{}
		server.bootstrapUntil = time.Time{}
		server.session = &sessionState{
			cookieHash: sha256.Sum256([]byte(sessionValue)),
			csrfHash:   sha256.Sum256([]byte(csrfValue)),
			csrfToken:  csrfBytes,
		}
	}
	server.mu.Unlock()
	if !valid {
		writeSafeError(response, http.StatusUnauthorized)
		return
	}

	http.SetCookie(response, &http.Cookie{
		Name:     server.sessionCookie,
		Value:    sessionValue,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(response).Encode(struct {
		CSRFToken string `json:"csrfToken"`
	}{CSRFToken: csrfValue})
}

func (server *Server) authenticated(request *http.Request) bool {
	cookies := request.CookiesNamed(server.sessionCookie)
	if len(cookies) != 1 || len(cookies[0].Value) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return false
	}
	presented := sha256.Sum256([]byte(cookies[0].Value))
	server.mu.Lock()
	defer server.mu.Unlock()
	return !server.closed && server.session != nil && subtle.ConstantTimeCompare(presented[:], server.session.cookieHash[:]) == 1
}

func (server *Server) serveCSRFRefresh(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeSafeError(response, http.StatusMethodNotAllowed)
		return
	}
	if request.URL.RawQuery != "" {
		writeSafeError(response, http.StatusBadRequest)
		return
	}
	// Unlike ordinary authenticated reads, this credential-bearing response is
	// never available to a top-level navigation or the local read-only CLI
	// profile. Sec-Fetch-* headers are browser-controlled, so this admits only a
	// same-origin fetch/XHR shape after validTransportRequest checked Host,
	// Origin, and the exact loopback peer.
	if !sameOriginBrowserRead(request.Header, false) {
		writeSafeError(response, http.StatusForbidden)
		return
	}

	server.mu.Lock()
	if server.closed || server.session == nil {
		server.mu.Unlock()
		writeSafeError(response, http.StatusUnauthorized)
		return
	}
	csrfToken := server.session.csrfToken
	server.mu.Unlock()

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(response).Encode(struct {
		CSRFToken string `json:"csrfToken"`
	}{CSRFToken: base64.RawURLEncoding.EncodeToString(csrfToken[:])})
}

func (server *Server) validCSRF(request *http.Request) bool {
	values := request.Header.Values(CSRFHeader)
	if len(values) != 1 || len(values[0]) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return false
	}
	presented := sha256.Sum256([]byte(values[0]))
	server.mu.Lock()
	defer server.mu.Unlock()
	return !server.closed && server.session != nil && subtle.ConstantTimeCompare(presented[:], server.session.csrfHash[:]) == 1
}

func needsCSRF(method string) bool {
	return method != http.MethodGet && method != http.MethodHead
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		setSecurityHeaders(response.Header())
		next.ServeHTTP(response, request)
	})
}

func setSecurityHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}

func recoverSafely(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		defer func() {
			if recover() != nil {
				writeSafeError(response, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(response, request)
	})
}

func writeSafeError(response http.ResponseWriter, status int) {
	clearHeaders(response.Header())
	setSecurityHeaders(response.Header())
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_, _ = io.WriteString(response, "{\"error\":\"request rejected\"}\n")
}

func clearHeaders(header http.Header) {
	for name := range header {
		header.Del(name)
	}
}

func normalizeConfig(config Config) (Config, error) {
	if config.MaxBodyBytes < 0 || config.MaxResponseBytes < 0 || config.RequestTimeout < 0 || config.ReadHeaderTimeout < 0 || config.ReadTimeout < 0 || config.WriteTimeout < 0 || config.IdleTimeout < 0 || config.BootstrapTTL < 0 {
		return Config{}, errors.New("local HTTP limits must not be negative")
	}
	if config.MaxBodyBytes == 0 {
		config.MaxBodyBytes = defaultMaxBodyBytes
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = defaultMaxResponse
	}
	if config.MaxBodyBytes > maximumBodyBytes || config.MaxResponseBytes > maximumResponse {
		return Config{}, errors.New("local HTTP body or response limit exceeds hard maximum")
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 10 * time.Second
	}
	if config.ReadHeaderTimeout == 0 {
		config.ReadHeaderTimeout = 2 * time.Second
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 10 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 15 * time.Second
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 30 * time.Second
	}
	if config.BootstrapTTL == 0 {
		config.BootstrapTTL = defaultBootstrapTTL
	}
	if config.RequestTimeout > 2*time.Minute || config.ReadHeaderTimeout > 10*time.Second || config.ReadTimeout > 2*time.Minute || config.WriteTimeout > 2*time.Minute || config.IdleTimeout > 5*time.Minute || config.BootstrapTTL > maximumBootstrapTTL {
		return Config{}, errors.New("local HTTP timeout exceeds hard maximum")
	}
	return config, nil
}
