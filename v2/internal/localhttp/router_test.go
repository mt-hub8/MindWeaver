package localhttp

import (
	"errors"
	"net/http"
	"testing"
)

func TestRouterAcceptsOnlyFixedBusinessRoutesAndSealsAtStart(t *testing.T) {
	router := NewRouter()
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for _, candidate := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/"},
		{method: http.MethodGet, path: HealthPath},
		{method: http.MethodGet, path: "/api/"},
		{method: http.MethodGet, path: "/api/items/"},
		{method: http.MethodGet, path: "/api/{id}"},
		{method: http.MethodGet, path: "/api/%69tems"},
		{method: http.MethodOptions, path: "/api/items"},
	} {
		if err := router.Handle(candidate.method, candidate.path, handler); !errors.Is(err, ErrInvalidRoute) {
			t.Errorf("Handle(%q, %q) error = %v", candidate.method, candidate.path, err)
		}
	}
	if err := router.Handle(http.MethodGet, "/api/items", handler); err != nil {
		t.Fatal(err)
	}
	if err := router.Handle(http.MethodGet, "/api/items", handler); !errors.Is(err, ErrDuplicateRoute) {
		t.Fatalf("duplicate error = %v", err)
	}
	active := startTestServer(t, router, Config{})
	if err := router.Handle(http.MethodPost, "/api/items", handler); !errors.Is(err, ErrRouterSealed) {
		t.Fatalf("post-Start registration error = %v", err)
	}
	if active.server == nil {
		t.Fatal("server was not started")
	}
}
