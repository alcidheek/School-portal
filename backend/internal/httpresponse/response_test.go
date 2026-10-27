package httpresponse

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorJSON(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, "sql: private server diagnostic", 500)
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if w.Code != 500 || response["error"] != "Internal server error" || len(response) != 1 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response: %d %v", w.Code, response)
	}
}

func TestDecodeOneObject(t *testing.T) {
	for _, body := range []string{"", "null", "[]", `"string"`, "{", `{} {}`, `{} trailing`, `{"name":42}`, strings.Repeat(" ", 1<<20) + `{}`} {
		t.Run(body[:min(len(body), 20)], func(t *testing.T) {
			var target struct {
				Name string `json:"name"`
			}
			if err := Decode(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(body)), &target); err == nil {
				t.Fatalf("accepted malformed/non-object request %q", body[:min(len(body), 20)])
			}
		})
	}
	var target struct {
		Name string `json:"name"`
	}
	if err := Decode(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"ok","extra":"compatible"}`)), &target); err != nil || target.Name != "ok" {
		t.Fatalf("valid decode: %v", err)
	}
}
