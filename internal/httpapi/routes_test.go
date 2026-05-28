package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" prod, edge ,, blue ")
	want := []string{"prod", "edge", "blue"}
	if len(got) != len(want) {
		t.Fatalf("splitCSV() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitCSV()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if got := splitCSV(""); got != nil {
		t.Fatalf("splitCSV(empty) = %#v, want nil", got)
	}
}

func TestPageSize(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int64
	}{
		{name: "default", query: "", want: 50},
		{name: "valid", query: "?page_size=25", want: 25},
		{name: "invalid", query: "?page_size=abc", want: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testContext("GET", "/v1/servers"+tt.query)
			if got := pageSize(ctx); got != tt.want {
				t.Fatalf("pageSize() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseOptionalTime(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ctx := testContext("GET", "/v1/results")
		got, ok := parseOptionalTime(ctx, "start_time")
		if !ok {
			t.Fatal("parseOptionalTime() ok = false, want true")
		}
		if got != nil {
			t.Fatalf("parseOptionalTime() = %v, want nil", got)
		}
	})

	t.Run("valid RFC3339", func(t *testing.T) {
		ctx := testContext("GET", "/v1/results?start_time=2026-05-29T01:22:00%2B08:00")
		got, ok := parseOptionalTime(ctx, "start_time")
		if !ok {
			t.Fatal("parseOptionalTime() ok = false, want true")
		}
		want := time.Date(2026, 5, 29, 1, 22, 0, 0, time.FixedZone("", 8*60*60))
		if !got.Equal(want) {
			t.Fatalf("parseOptionalTime() = %v, want %v", got, want)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		ctx := testContext("GET", "/v1/results?start_time=not-a-time")
		got, ok := parseOptionalTime(ctx, "start_time")
		if ok {
			t.Fatal("parseOptionalTime() ok = true, want false")
		}
		if got != nil {
			t.Fatalf("parseOptionalTime() = %v, want nil", got)
		}
		if ctx.Writer.Status() != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", ctx.Writer.Status(), http.StatusBadRequest)
		}
	})
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "missing"},
		{name: "bearer", header: "Bearer abc123", want: "abc123"},
		{name: "case insensitive prefix", header: "bearer token", want: "token"},
		{name: "trims token", header: "Bearer   spaced-token  ", want: "spaced-token"},
		{name: "wrong scheme", header: "Basic abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testContext("GET", "/v1/auth/me")
			if tt.header != "" {
				ctx.Request.Header.Set("Authorization", tt.header)
			}
			if got := bearerToken(ctx); got != tt.want {
				t.Fatalf("bearerToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func testContext(method, target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(method, target, nil)
	return ctx
}
