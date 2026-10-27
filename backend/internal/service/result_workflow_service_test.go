package service

import (
	"context"
	"errors"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
	"math"
	"testing"
)

func TestDraftScoreValidation(t *testing.T) {
	for _, tc := range []struct {
		ca, exam float64
		valid    bool
	}{{0, 0, true}, {40, 60, true}, {33.25, 55.5, true}, {-1, 1, false}, {41, 0, false}, {0, 61, false}, {0, -1, false}, {math.NaN(), 0, false}, {0, math.Inf(1), false}, {1.001, 2, false}} {
		err := validateResultScores(tc.ca, tc.exam)
		if (err == nil) != tc.valid {
			t.Fatalf("scores %v/%v: %v", tc.ca, tc.exam, err)
		}
	}
}
func TestResultWorkflowInputValidationBeforeStorage(t *testing.T) {
	s := &ResultService{}
	ctx := context.Background()
	actor := uuid.New()
	if _, err := s.PublishClassTerm(ctx, actor, model.ResultPeriod{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	p := model.ResultPeriod{ClassID: uuid.New(), AcademicSessionID: uuid.New(), TermID: uuid.New(), SubjectID: uuid.New()}
	for _, tc := range []struct{ action, reason string }{{"rejected", " "}, {"published", ""}, {"approved_individual", ""}} {
		if err := s.TransitionBatch(ctx, actor, p, tc.action, tc.reason); !errors.Is(err, ErrInvalidInput) {
			t.Fatal(err)
		}
	}
	if _, err := s.PublishClassTerm(ctx, actor, p); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("subject publication accepted", err)
	}
}
