package handler_test

import (
	"net/http"
	"testing"
)

// TestNamesRoutesRequireAdmin keeps the boilerplate CRUD surface behind the
// admin role: every API route must authenticate and authorize.
func TestNamesRoutesRequireAdmin(t *testing.T) {
	api := newClaimRouter(t)
	admin := loginAs(t, api, "admin-demo")
	finder := loginAs(t, api, "finder-demo")

	cases := []struct {
		method string
		path   string
		body   string
		ok     int
	}{
		{http.MethodGet, "/names/1", "", http.StatusOK},
		{http.MethodPost, "/names", `{"name":"Alice"}`, http.StatusCreated},
		{http.MethodPut, "/names/1", `{"name":"Alice"}`, http.StatusOK},
		{http.MethodDelete, "/names/1", "", http.StatusNoContent},
		{http.MethodPost, "/names/1/replace", `{"name":"Alice"}`, http.StatusCreated},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			if got := doRequest(t, api, tc.method, tc.path, tc.body, nil).Code; got != http.StatusUnauthorized {
				t.Fatalf("unauthenticated status = %d, want 401", got)
			}
			if got := doRequest(t, api, tc.method, tc.path, tc.body, finder).Code; got != http.StatusForbidden {
				t.Fatalf("finder status = %d, want 403", got)
			}
			if got := doRequest(t, api, tc.method, tc.path, tc.body, admin).Code; got != tc.ok {
				t.Fatalf("admin status = %d, want %d", got, tc.ok)
			}
		})
	}
}
