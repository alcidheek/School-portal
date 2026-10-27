package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func TestAcademicErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"validation", fmt.Errorf("%w: invalid term start date", service.ErrInvalidAcademicInput), 400, "invalid term start date"},
		{"missing session", service.ErrAcademicSessionNotFound, 404, "academic session not found"},
		{"missing active term", service.ErrActiveTermNotFound, 404, "active term not found"},
		{"inactive", service.ErrAcademicSessionInactive, 409, "academic session is not active"},
		{"no terms", service.ErrNoAvailableTerm, 409, "no available term"},
		{"dates conflict", service.ErrTermDatesConflict, 409, "conflict with existing term dates"},
		{"wrong session", service.ErrTermSessionMismatch, 409, "does not belong"},
		{"database conflict", fmt.Errorf("%w: %w", service.ErrAcademicConflict, &pq.Error{Code: "23505", Message: "private SQL detail"}), 409, "academic lifecycle conflict"},
		{"internal", errors.New("private SQL detail"), 500, "Internal server error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeAcademicError(w, tc.err)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.message) || strings.Contains(w.Body.String(), "private SQL") {
				t.Fatalf("response: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestAcademicHandlersInternalFailures(t *testing.T) {
	// No server connection is made: a closed pool produces a real storage failure.
	db, err := sql.Open("postgres", "postgres://postgres@127.0.0.1:1/postgres?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	h := NewAcademicSessionHandler(service.NewAcademicSessionService(repository.NewAcademicSessionRepository(db), repository.NewTermRepository(db)))
	for _, tc := range []struct {
		name, body string
		call       http.HandlerFunc
	}{
		{"create", `{"name":"A","start_date":"2026-01-01","end_date":"2026-12-31","activate":true,"term_name":"FIRST_TERM","term_start_date":"2026-01-01","term_end_date":"2026-04-01"}`, h.Create},
		{"switch", `{"term_name":"FIRST_TERM","start_date":"2026-01-01","end_date":"2026-04-01"}`, h.SwitchTerm},
		{"activate", "", h.Activate}, {"get active", "", h.GetActiveTerm}, {"list", "", h.List}, {"list terms", "", h.ListTerms},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("PATCH", "/", strings.NewReader(tc.body))
			r.SetPathValue("id", uuid.NewString())
			w := httptest.NewRecorder()
			tc.call(w, r)
			if w.Code != 500 || strings.Contains(w.Body.String(), "database is closed") {
				t.Fatalf("storage error exposed: %d %s", w.Code, w.Body)
			}
		})
	}
	for _, call := range []http.HandlerFunc{h.Activate, h.SwitchTerm, h.GetActiveTerm, h.ListTerms} {
		r := httptest.NewRequest("PATCH", "/", nil)
		r.SetPathValue("id", "bad")
		w := httptest.NewRecorder()
		call(w, r)
		if w.Code != 400 {
			t.Fatalf("invalid UUID status: %d", w.Code)
		}
	}
	for _, call := range []http.HandlerFunc{h.Create, h.SwitchTerm} {
		r := httptest.NewRequest("PATCH", "/", strings.NewReader("{"))
		r.SetPathValue("id", uuid.NewString())
		w := httptest.NewRecorder()
		call(w, r)
		if w.Code != 400 {
			t.Fatalf("malformed body status: %d", w.Code)
		}
	}
}

func TestAcademicHTTPContracts(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL academic handler tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, name := range []string{"002_academic_section.sql", "003_terms.sql"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("CREATE TEMP TABLE" + string(content[len("CREATE TABLE"):])); err != nil {
			t.Fatal(err)
		}
	}
	content, err := os.ReadFile(filepath.Join("..", "..", "migrations", "012_academic_lifecycle.sql"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(string(content)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	h := NewAcademicSessionHandler(service.NewAcademicSessionService(repository.NewAcademicSessionRepository(db), repository.NewTermRepository(db)))
	call := func(fn http.HandlerFunc, method, body, id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/", strings.NewReader(body))
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		fn(w, r)
		return w
	}
	createBody := `{"name":"A","start_date":"2026-01-01","end_date":"2026-12-31","activate":true,"term_name":"FIRST_TERM","term_start_date":"2026-01-01","term_end_date":"2026-04-01"}`
	w := call(h.Create, "POST", createBody, "")
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var session map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "name", "start_date", "end_date", "is_active", "created_at", "updated_at"} {
		if _, ok := session[key]; !ok {
			t.Fatalf("missing session response field %s", key)
		}
	}
	if len(session) != 7 {
		t.Fatalf("session response changed: %v", session)
	}
	id := session["id"].(string)
	w = call(h.Activate, "PATCH", "", id)
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("activate response: %d %s", w.Code, w.Body)
	}
	w = call(h.GetActiveTerm, "GET", "", id)
	if w.Code != 200 {
		t.Fatalf("active term: %d %s", w.Code, w.Body)
	}
	var original map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	switchBody := `{"term_name":"FIRST_TERM","start_date":"2026-01-01","end_date":"2026-04-01"}`
	w = call(h.SwitchTerm, "PATCH", switchBody, id)
	var reused map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &reused); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(reused) != 8 || reused["id"] != original["id"] || reused["is_active"] != true {
		t.Fatalf("switch response changed: %d %v", w.Code, reused)
	}
	if w := call(h.Create, "POST", createBody, ""); w.Code != 409 || strings.Contains(w.Body.String(), "duplicate key") {
		t.Fatalf("duplicate creation: %d %s", w.Code, w.Body)
	}
	for _, fn := range []http.HandlerFunc{h.Activate, h.GetActiveTerm, h.ListTerms} {
		if w := call(fn, "PATCH", "", uuid.NewString()); w.Code != 404 {
			t.Fatalf("missing session: %d %s", w.Code, w.Body)
		}
	}
	if w := call(h.SwitchTerm, "PATCH", strings.Replace(switchBody, "2026-01-01", "2026-01-02", 1), id); w.Code != 409 {
		t.Fatalf("date conflict: %d %s", w.Code, w.Body)
	}
	if w := call(h.SwitchTerm, "PATCH", strings.Replace(switchBody, "2026-01-01", "2025-01-01", 1), id); w.Code != 400 {
		t.Fatalf("invalid dates: %d %s", w.Code, w.Body)
	}
	t.Run("list existing terms without changing records", func(t *testing.T) {
		secondTermResponse := call(h.SwitchTerm, "PATCH", `{"term_name":"SECOND_TERM","start_date":"2026-05-01","end_date":"2026-08-01"}`, id)
		if secondTermResponse.Code != http.StatusOK {
			t.Fatalf("create second term: %d %s", secondTermResponse.Code, secondTermResponse.Body)
		}
		var secondTerm map[string]any
		if err := json.Unmarshal(secondTermResponse.Body.Bytes(), &secondTerm); err != nil {
			t.Fatal(err)
		}
		original["is_active"] = false
		otherSessionID := uuid.NewString()
		emptySessionID := uuid.NewString()
		if _, err := db.Exec(`INSERT INTO academic_sessions(id,name,start_date,end_date) VALUES($1,'Other','2026-01-01','2026-12-31'),($2,'Empty','2026-01-01','2026-12-31')`, otherSessionID, emptySessionID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO terms(academic_session_id,name,start_date,end_date) VALUES($1,'THIRD_TERM','2026-09-01','2026-12-01')`, otherSessionID); err != nil {
			t.Fatal(err)
		}
		var termsBefore string
		if err := db.QueryRow(`SELECT json_agg(terms ORDER BY id)::text FROM terms`).Scan(&termsBefore); err != nil {
			t.Fatal(err)
		}
		listResponse := call(h.ListTerms, "GET", "", id)
		if listResponse.Code != http.StatusOK || listResponse.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("list terms: %d %s", listResponse.Code, listResponse.Body)
		}
		var listedTerms []map[string]any
		if err := json.Unmarshal(listResponse.Body.Bytes(), &listedTerms); err != nil {
			t.Fatal(err)
		}
		if len(listedTerms) != 2 {
			t.Fatalf("expected only two existing session terms, got %v", listedTerms)
		}
		for termIndex, expectedTerm := range []map[string]any{original, secondTerm} {
			for _, field := range []string{"id", "academic_session_id", "name", "start_date", "end_date", "is_active"} {
				if listedTerms[termIndex][field] != expectedTerm[field] {
					t.Errorf("term %d field %s: got %v, want %v", termIndex, field, listedTerms[termIndex][field], expectedTerm[field])
				}
			}
			if listedTerms[termIndex]["academic_session_id"] != id {
				t.Errorf("term belongs to another session: %v", listedTerms[termIndex])
			}
		}
		emptyResponse := call(h.ListTerms, "GET", "", emptySessionID)
		if emptyResponse.Code != http.StatusOK || strings.TrimSpace(emptyResponse.Body.String()) != "[]" {
			t.Fatalf("empty terms: %d %s", emptyResponse.Code, emptyResponse.Body)
		}
		inactiveResponse := call(h.ListTerms, "GET", "", otherSessionID)
		if inactiveResponse.Code != http.StatusOK || !strings.Contains(inactiveResponse.Body.String(), "THIRD_TERM") {
			t.Fatalf("inactive session terms: %d %s", inactiveResponse.Code, inactiveResponse.Body)
		}
		var termsAfter string
		if err := db.QueryRow(`SELECT json_agg(terms ORDER BY id)::text FROM terms`).Scan(&termsAfter); err != nil {
			t.Fatal(err)
		}
		if termsAfter != termsBefore {
			t.Error("listing changed stored terms")
		}
	})
	if _, err := db.Exec(`UPDATE terms SET is_active=FALSE; UPDATE academic_sessions SET is_active=FALSE`); err != nil {
		t.Fatal(err)
	}
	if w := call(h.SwitchTerm, "PATCH", switchBody, id); w.Code != 409 {
		t.Fatalf("inactive switch: %d %s", w.Code, w.Body)
	}
	if w := call(h.GetActiveTerm, "GET", "", id); w.Code != 404 {
		t.Fatalf("missing active term: %d %s", w.Code, w.Body)
	}
	if w := call(h.List, "GET", "", ""); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "[") {
		t.Fatalf("list response: %d %s", w.Code, w.Body)
	}
}
