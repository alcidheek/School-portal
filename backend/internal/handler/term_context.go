package handler

import (
	"github.com/google/uuid"
	"net/http"
)

func assignmentTerm(r *http.Request) uuid.UUID {
	values := r.URL.Query()["term_id"]
	if len(values) != 1 {
		return uuid.Nil
	}
	id, err := uuid.Parse(values[0])
	if err != nil {
		return uuid.Nil
	}
	return id
}
