package cmd

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestWithAuth(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})

	t.Run("disabled", func(t *testing.T) {
		t.Setenv(passwordEnv, "")
		t.Setenv(publicURLEnv, "")
		h, err := withAuth(ok)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("partial", func(t *testing.T) {
		t.Setenv(passwordEnv, "secret")
		t.Setenv(publicURLEnv, "")
		if _, err := withAuth(ok); err == nil {
			t.Fatal("want error when only the password is set")
		}
	})

	t.Run("enabled", func(t *testing.T) {
		t.Setenv(passwordEnv, "secret")
		t.Setenv(publicURLEnv, "http://localhost:8080")
		t.Setenv(stateFileEnv, filepath.Join(t.TempDir(), "state.json"))
		t.Setenv(redirectURIsEnv, "https://agent.meta.ai/api/hatch/oauth/callback")
		h, err := withAuth(ok)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}

func TestSplitList(t *testing.T) {
	got := splitList(" a, ,b ,")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitList = %q", got)
	}
}
