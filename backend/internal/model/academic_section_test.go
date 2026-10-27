package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAcademicSessionRememberedTermIsInternal(t *testing.T) {
	id := uuid.New()
	session := AcademicSession{LastActiveTermID: uuid.NullUUID{UUID: id, Valid: true}}
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "last_active_term") || strings.Contains(string(data), id.String()) {
		t.Fatalf("remembered term leaked into API: %s", data)
	}
	var decoded AcademicSession
	if err := json.Unmarshal([]byte(`{"last_active_term_id":"`+id.String()+`"}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.LastActiveTermID.Valid {
		t.Fatal("API input populated internal pointer")
	}
}
