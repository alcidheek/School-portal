package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const promotionDecisionsPath = "/api/v1/admin/student-promotions/decisions"
const promotionFinalizePath = "/api/v1/admin/student-promotions/finalize"

func TestPromotionDeleteSavedDecision(t *testing.T) {
	fixture := newPromotionFixture(t)
	expectOwnershipStatus(t, fixture.draft(t, "teacherA", "studentA", "math", 30, 50), http.StatusCreated)
	if _, err := fixture.db.Exec(`INSERT INTO attendance(student_id,class_id,academic_session_id,term_id,date,status,recorded_by)
 VALUES($1,$2,$3,$4,'2026-02-02','PRESENT',$5)`, fixture.users["studentA"].ID, fixture.ids["classA"], fixture.ids["sessionA"], fixture.ids["termA"], fixture.users["teacherA"].ID); err != nil {
		t.Fatal(err)
	}
	decision := savePromotion(t, fixture, "studentA", model.PromotionDecisionPromote)
	other := savePromotion(t, fixture, "studentB", model.PromotionDecisionRepeat)
	const historyQuery = `SELECT json_build_object(
 'enrollments',(SELECT json_agg(row_to_json(e) ORDER BY id) FROM student_enrollments e),
 'results',(SELECT json_agg(row_to_json(r) ORDER BY id) FROM results r),
 'attendance',(SELECT json_agg(row_to_json(a) ORDER BY id) FROM attendance a),
 'users',(SELECT json_agg(row_to_json(u) ORDER BY id) FROM users u))::text`
	var before, after string
	if err := fixture.db.QueryRow(historyQuery).Scan(&before); err != nil {
		t.Fatal(err)
	}
	response := fixture.call(t, "admin", http.MethodDelete, promotionDecisionsPath+"/"+decision.ID.String(), "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("status %d want 204: %s", response.Code, response.Body)
	}
	if response.Body.Len() != 0 {
		t.Fatal("204 response must have no body")
	}
	var remaining, audits int
	if err := fixture.db.QueryRow(`SELECT count(*) FROM student_promotion_decisions WHERE id=$1`, decision.ID).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("decision was not deleted: %d %v", remaining, err)
	}
	if err := fixture.db.QueryRow(`SELECT count(*) FROM student_promotion_decisions WHERE id=$1`, other.ID).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("unrelated decision changed: %d %v", remaining, err)
	}
	if err := fixture.db.QueryRow(`SELECT count(*) FROM term_configuration_events WHERE action='promotion.delete'
 AND actor_id=$1 AND term_id=$2 AND old_record->>'id'=$3 AND old_record->>'status'='SAVED' AND new_record='null'::jsonb`, fixture.users["admin"].ID, decision.SourceTermID, decision.ID.String()).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("deletion was not audited: %d %v", audits, err)
	}
	if err := fixture.db.QueryRow(historyQuery).Scan(&after); err != nil || before != after {
		t.Fatalf("academic records or admission numbers changed: %v", err)
	}
}

func TestPromotionDeleteFinalizedDecisionRejected(t *testing.T) {
	fixture := newPromotionFixture(t)
	decision := savePromotion(t, fixture, "studentA", model.PromotionDecisionPromote)
	activatePromotionSession(t, fixture, fixture.ids["destinationSession"])
	if outcome := finalizePromotionBatch(t, fixture, http.StatusOK, decision.ID); outcome.Succeeded != 1 {
		t.Fatalf("finalization failed: %+v", outcome)
	}
	expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, promotionDecisionsPath+"/"+decision.ID.String(), ""), http.StatusConflict)
	var status string
	if err := fixture.db.QueryRow(`SELECT status FROM student_promotion_decisions WHERE id=$1`, decision.ID).Scan(&status); err != nil || status != model.PromotionStatusFinalized {
		t.Fatalf("finalized decision changed: %s %v", status, err)
	}
	if _, err := fixture.db.Exec(`DELETE FROM student_promotion_decisions WHERE id=$1`, decision.ID); err == nil {
		t.Fatal("database allowed finalized decision deletion")
	}
}

func TestPromotionDeleteUnauthorized(t *testing.T) {
	fixture := newPromotionFixture(t)
	decision := savePromotion(t, fixture, "studentA", model.PromotionDecisionPromote)
	for _, actor := range []string{"", "teacherA", "studentA", "inactive"} {
		status := http.StatusForbidden
		if actor == "" {
			status = http.StatusUnauthorized
		}
		expectOwnershipStatus(t, fixture.call(t, actor, http.MethodDelete, promotionDecisionsPath+"/"+decision.ID.String(), ""), status)
	}
	if err := fixture.results.DeletePromotionDecision(context.Background(), fixture.users["teacherA"].ID, decision.ID); err == nil {
		t.Fatal("service allowed teacher deletion")
	}
	var remaining int
	if err := fixture.db.QueryRow(`SELECT count(*) FROM student_promotion_decisions WHERE id=$1`, decision.ID).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("unauthorized request deleted decision: %d %v", remaining, err)
	}
}

func TestPromotionDeleteNonexistentDecision(t *testing.T) {
	fixture := newPromotionFixture(t)
	expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, promotionDecisionsPath+"/"+uuid.New().String(), ""), http.StatusNotFound)
	expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, promotionDecisionsPath+"/invalid", ""), http.StatusBadRequest)
}

func TestPromotionDeleteAuditFailureRollsBack(t *testing.T) {
	fixture := newPromotionFixture(t)
	decision := savePromotion(t, fixture, "studentA", model.PromotionDecisionPromote)
	if _, err := fixture.db.Exec(`CREATE FUNCTION reject_promotion_delete_audit() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN RAISE EXCEPTION 'private database failure'; END $$;
 CREATE TRIGGER reject_promotion_delete_audit BEFORE INSERT ON term_configuration_events
 FOR EACH ROW EXECUTE FUNCTION reject_promotion_delete_audit()`); err != nil {
		t.Fatal(err)
	}
	expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, promotionDecisionsPath+"/"+decision.ID.String(), ""), http.StatusInternalServerError)
	var remaining int
	if err := fixture.db.QueryRow(`SELECT count(*) FROM student_promotion_decisions WHERE id=$1`, decision.ID).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("audit failure did not roll back deletion: %d %v", remaining, err)
	}
}

func activatePromotionSession(t *testing.T, fixture *ownershipFixture, sessionID uuid.UUID) {
	t.Helper()
	academicService := service.NewAcademicSessionService(
		repository.NewAcademicSessionRepository(fixture.db), repository.NewTermRepository(fixture.db))
	if err := academicService.Activate(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
}

func newPromotionFixture(t *testing.T) *ownershipFixture {
	t.Helper()
	fixture := newOwnershipFixture(t)
	migration, err := os.ReadFile("../migrations/017_student_promotion_decisions.sql")
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := fixture.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	migration, err = os.ReadFile("../migrations/018_allow_saved_promotion_decision_deletion.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	fixture.ids["destinationSession"] = uuid.New()
	fixture.ids["destinationTerm"] = uuid.New()
	if _, err := fixture.db.Exec(`INSERT INTO academic_sessions(id,name,start_date,end_date)
 VALUES($1,'2027 promotion destination','2027-01-01','2027-12-31')`, fixture.ids["destinationSession"]); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec(`INSERT INTO terms(id,academic_session_id,name,start_date,end_date)
 VALUES($1,$2,'FIRST_TERM','2027-01-01','2027-04-01')`, fixture.ids["destinationTerm"], fixture.ids["destinationSession"]); err != nil {
		t.Fatal(err)
	}
	activatePromotionSession(t, fixture, fixture.ids["sessionA"])
	return fixture
}

func promotionSaveRequest(fixture *ownershipFixture, studentName, decision string) model.SaveStudentPromotionDecisionsRequest {
	className := "classA"
	if studentName == "studentB" {
		className = "classB"
	}
	input := model.StudentPromotionDecisionInput{StudentID: fixture.users[studentName].ID, Decision: decision}
	if decision == model.PromotionDecisionPromote {
		targetClassID := fixture.ids["emptyClass"]
		input.TargetClassID = &targetClassID
	}
	return model.SaveStudentPromotionDecisionsRequest{
		PromotionSource: model.PromotionSource{
			SourceClassID: fixture.ids[className], SourceAcademicSessionID: fixture.ids["sessionA"], SourceTermID: fixture.ids["termA"],
		},
		TargetSessionStartDate: "2027-01-01",
		Decisions:              []model.StudentPromotionDecisionInput{input},
	}
}

func savePromotion(t *testing.T, fixture *ownershipFixture, studentName, decision string) model.StudentPromotionDecision {
	t.Helper()
	request := promotionSaveRequest(fixture, studentName, decision)
	response := fixture.call(t, "admin", http.MethodPut, promotionDecisionsPath, workflowJSON(t, request))
	expectOwnershipStatus(t, response, http.StatusOK)
	var saved model.SaveStudentPromotionDecisionsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Decisions) != 1 {
		t.Fatalf("expected one decision: %+v", saved)
	}
	return saved.Decisions[0]
}

func promotionFinalizeRequest(fixture *ownershipFixture, decisionIDs ...uuid.UUID) model.FinalizeStudentPromotionsRequest {
	return model.FinalizeStudentPromotionsRequest{
		AcademicSessionID: fixture.ids["destinationSession"], TermID: fixture.ids["destinationTerm"], DecisionIDs: decisionIDs,
	}
}

func finalizePromotionBatch(t *testing.T, fixture *ownershipFixture, expectedStatus int, decisionIDs ...uuid.UUID) model.FinalizeStudentPromotionsResponse {
	t.Helper()
	request := promotionFinalizeRequest(fixture, decisionIDs...)
	response := fixture.call(t, "admin", http.MethodPost, promotionFinalizePath, workflowJSON(t, request))
	expectOwnershipStatus(t, response, expectedStatus)
	var outcomes model.FinalizeStudentPromotionsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &outcomes); err != nil {
		t.Fatal(err)
	}
	if outcomes.Total != len(decisionIDs) || len(outcomes.Results) != len(decisionIDs) {
		t.Fatalf("missing individual outcomes: %+v", outcomes)
	}
	if outcomes.Succeeded+outcomes.AlreadyFinalized+outcomes.Failed != outcomes.Total {
		t.Fatalf("incorrect batch counts: %+v", outcomes)
	}
	return outcomes
}

func TestPromotionSaveDecisionsAndUpdate(t *testing.T) {
	fixture := newPromotionFixture(t)
	var decisionID uuid.UUID
	for _, decision := range []string{model.PromotionDecisionPromote, model.PromotionDecisionRepeat, model.PromotionDecisionPending} {
		request := promotionSaveRequest(fixture, "studentA", decision)
		response, err := fixture.results.SavePromotionDecisions(context.Background(), fixture.users["admin"].ID, request)
		if err != nil {
			t.Fatal(err)
		}
		saved := response.Decisions[0]
		if decisionID == uuid.Nil {
			decisionID = saved.ID
			if response.Saved != 1 || response.Updated != 0 {
				t.Fatalf("incorrect insert counts: %+v", response)
			}
		} else if saved.ID != decisionID || response.Updated != 1 || response.Saved != 0 {
			t.Fatalf("update created a new decision: %+v", response)
		}
		if saved.Decision != decision || saved.Status != model.PromotionStatusSaved || saved.TargetAcademicSessionID != nil {
			t.Fatalf("incorrect saved decision: %+v", saved)
		}
		if decision == model.PromotionDecisionRepeat && (saved.TargetClassID == nil || *saved.TargetClassID != request.SourceClassID) {
			t.Fatal("repeat did not retain the source class")
		}
		if countTerm(t, fixture, "student_enrollments", fixture.ids["destinationTerm"]) != 0 || countTerm(t, fixture, "student_enrollments", fixture.ids["termA"]) != 2 {
			t.Fatal("saving changed enrollment")
		}
	}
	query := fmt.Sprintf("?source_class_id=%s&source_academic_session_id=%s&source_term_id=%s", fixture.ids["classA"], fixture.ids["sessionA"], fixture.ids["termA"])
	response := fixture.call(t, "admin", http.MethodGet, promotionDecisionsPath+query, "")
	expectOwnershipStatus(t, response, http.StatusOK)
	var listed struct {
		Decisions []model.StudentPromotionDecision `json:"decisions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil || len(listed.Decisions) != 1 || listed.Decisions[0].StudentName != "studentA" {
		t.Fatalf("incorrect list: %s, %v", response.Body, err)
	}
}

func TestPromotionDecisionsAllowedAfterSourceResultsSubmitted(t *testing.T) {
	fixture := newPromotionFixture(t)
	sourcePeriod := fixture.period("math")
	expectOwnershipStatus(t, fixture.draft(t, "teacherA", "studentA", "math", 30, 50), http.StatusCreated)
	if err := fixture.results.TransitionBatch(context.Background(), fixture.users["teacherA"].ID, sourcePeriod, "submitted", ""); err != nil {
		t.Fatal(err)
	}
	configuration, err := fixture.results.TermConfiguration(context.Background(), fixture.users["admin"].ID, sourcePeriod, nil)
	if err != nil || !configuration.Locked {
		t.Fatalf("source configuration must be locked: %+v %v", configuration, err)
	}
	query := fmt.Sprintf("?source_class_id=%s&source_academic_session_id=%s&source_term_id=%s",
		sourcePeriod.ClassID, sourcePeriod.AcademicSessionID, sourcePeriod.TermID)
	// Listing an empty or saved decision set must work after exams are submitted.
	expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodGet, promotionDecisionsPath+query, ""), http.StatusOK)
	const sourceSnapshotQuery = `SELECT json_build_object(
 'enrollments',(SELECT json_agg(row_to_json(enrollment) ORDER BY id) FROM student_enrollments enrollment WHERE term_id=$1),
 'results',(SELECT json_agg(row_to_json(result) ORDER BY id) FROM results result),
 'batches',(SELECT json_agg(row_to_json(batch) ORDER BY class_id,subject_id) FROM result_batches batch),
 'events',(SELECT json_agg(row_to_json(event) ORDER BY id) FROM result_batch_events event))::text`
	var sourceBefore, sourceAfter string
	if err := fixture.db.QueryRow(sourceSnapshotQuery, sourcePeriod.TermID).Scan(&sourceBefore); err != nil {
		t.Fatal(err)
	}
	savedDecision := savePromotion(t, fixture, "studentA", model.PromotionDecisionPromote)
	updatedDecision := savePromotion(t, fixture, "studentA", model.PromotionDecisionRepeat)
	if savedDecision.ID != updatedDecision.ID || updatedDecision.Decision != model.PromotionDecisionRepeat {
		t.Fatal("existing decision was not updated")
	}
	response := fixture.call(t, "admin", http.MethodGet, promotionDecisionsPath+query, "")
	expectOwnershipStatus(t, response, http.StatusOK)
	var listed struct {
		Decisions []model.StudentPromotionDecision `json:"decisions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil || len(listed.Decisions) != 1 || listed.Decisions[0].Decision != model.PromotionDecisionRepeat {
		t.Fatalf("updated decision was not listed: %s %v", response.Body, err)
	}
	if err := fixture.db.QueryRow(sourceSnapshotQuery, sourcePeriod.TermID).Scan(&sourceAfter); err != nil || sourceBefore != sourceAfter {
		t.Fatalf("promotion changed source enrollment or submitted results: %v", err)
	}
	var auditCount int
	if err := fixture.db.QueryRow(`SELECT count(*) FROM term_configuration_events WHERE term_id=$1 AND action='promotion.save'`, sourcePeriod.TermID).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("promotion save and update must be audited: %d %v", auditCount, err)
	}
	// The same source must still reject actual enrollment mutations with P6001.
	_, err = fixture.db.Exec(`INSERT INTO student_enrollments(student_id,class_id,academic_session_id,term_id) VALUES($1,$2,$3,$4)`,
		fixture.users["studentB"].ID, sourcePeriod.ClassID, sourcePeriod.AcademicSessionID, sourcePeriod.TermID)
	var databaseError *pq.Error
	if !errors.As(err, &databaseError) || databaseError.Code != "P6001" {
		t.Fatalf("locked source enrollment guard was weakened: %v", err)
	}
	t.Logf("Enrollment mutation raises %s from %s: %s; promotion GET and PUT both succeed",
		databaseError.Code, databaseError.Where, databaseError.Message)
}

func TestPromotionAuthorizationAndInvalidRequests(t *testing.T) {
	fixture := newPromotionFixture(t)
	request := promotionSaveRequest(fixture, "studentA", model.PromotionDecisionPromote)
	for _, actor := range []string{"", "teacherA", "studentA", "inactive"} {
		expectedStatus := http.StatusForbidden
		if actor == "" {
			expectedStatus = http.StatusUnauthorized
		}
		for _, endpoint := range []struct{ method, path, body string }{
			{http.MethodPut, promotionDecisionsPath, workflowJSON(t, request)},
			{http.MethodGet, promotionDecisionsPath, ""},
			{http.MethodPost, promotionFinalizePath, workflowJSON(t, promotionFinalizeRequest(fixture, uuid.New()))},
		} {
			expectOwnershipStatus(t, fixture.call(t, actor, endpoint.method, endpoint.path, endpoint.body), expectedStatus)
		}
	}
	if _, err := fixture.results.SavePromotionDecisions(context.Background(), fixture.users["teacherA"].ID, request); err == nil {
		t.Fatal("service allowed a teacher to save decisions")
	}
	if _, err := fixture.results.FinalizePromotions(context.Background(), fixture.users["teacherA"].ID, promotionFinalizeRequest(fixture, uuid.New())); err == nil {
		t.Fatal("service allowed a teacher to finalize decisions")
	}
	if _, err := fixture.results.ListPromotionDecisions(context.Background(), fixture.users["teacherA"].ID, request.PromotionSource); err == nil {
		t.Fatal("service allowed a teacher to list decisions")
	}
	for _, testCase := range []struct {
		name   string
		change func(*model.SaveStudentPromotionDecisionsRequest)
		status int
	}{
		{"not enrolled", func(request *model.SaveStudentPromotionDecisionsRequest) {
			request.Decisions[0].StudentID = fixture.users["studentB"].ID
		}, 409},
		{"wrong source session", func(request *model.SaveStudentPromotionDecisionsRequest) {
			request.SourceAcademicSessionID = fixture.ids["sessionB"]
		}, 409},
		{"invalid decision", func(request *model.SaveStudentPromotionDecisionsRequest) { request.Decisions[0].Decision = "PASS" }, 400},
		{"missing target class", func(request *model.SaveStudentPromotionDecisionsRequest) { request.Decisions[0].TargetClassID = nil }, 400},
		{"invalid date", func(request *model.SaveStudentPromotionDecisionsRequest) {
			request.TargetSessionStartDate = "next year"
		}, 400},
		{"overlapping destination", func(request *model.SaveStudentPromotionDecisionsRequest) {
			request.TargetSessionStartDate = "2026-02-01"
		}, 409},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := promotionSaveRequest(fixture, "studentA", model.PromotionDecisionPromote)
			testCase.change(&request)
			expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodPut, promotionDecisionsPath, workflowJSON(t, request)), testCase.status)
		})
	}
}

func TestPromotionFinalizeAndPreserveHistory(t *testing.T) {
	fixture := newPromotionFixture(t)
	expectOwnershipStatus(t, fixture.draft(t, "teacherA", "studentA", "math", 30, 50), http.StatusCreated)
	if _, err := fixture.db.Exec(`INSERT INTO attendance(student_id,class_id,academic_session_id,term_id,date,status,recorded_by)
 VALUES($1,$2,$3,$4,'2026-02-02','PRESENT',$5)`, fixture.users["studentA"].ID, fixture.ids["classA"], fixture.ids["sessionA"], fixture.ids["termA"], fixture.users["teacherA"].ID); err != nil {
		t.Fatal(err)
	}
	const historyQuery = `SELECT json_build_object(
 'enrollments',(SELECT json_agg(row_to_json(enrollment) ORDER BY id) FROM student_enrollments enrollment WHERE term_id=$1),
 'results',(SELECT json_agg(row_to_json(result) ORDER BY id) FROM results result),
 'attendance',(SELECT json_agg(row_to_json(record) ORDER BY id) FROM attendance record),
 'admissions',(SELECT json_agg(json_build_array(id,admission_number) ORDER BY id) FROM users))::text`
	var historyBefore, historyAfter string
	if err := fixture.db.QueryRow(historyQuery, fixture.ids["termA"]).Scan(&historyBefore); err != nil {
		t.Fatal(err)
	}
	promoted := savePromotion(t, fixture, "studentA", model.PromotionDecisionPromote)
	repeated := savePromotion(t, fixture, "studentB", model.PromotionDecisionRepeat)
	activatePromotionSession(t, fixture, fixture.ids["destinationSession"])
	outcomes := finalizePromotionBatch(t, fixture, http.StatusOK, promoted.ID, repeated.ID)
	if outcomes.Succeeded != 2 {
		t.Fatalf("finalization failed: %+v", outcomes)
	}
	for _, placement := range []struct {
		student string
		classID uuid.UUID
	}{
		{"studentA", fixture.ids["emptyClass"]}, {"studentB", fixture.ids["classB"]},
	} {
		var classID uuid.UUID
		if err := fixture.db.QueryRow(`SELECT class_id FROM student_enrollments WHERE student_id=$1 AND term_id=$2`, fixture.users[placement.student].ID, fixture.ids["destinationTerm"]).Scan(&classID); err != nil || classID != placement.classID {
			t.Fatalf("incorrect placement: %s %s %v", placement.student, classID, err)
		}
	}
	outcomes = finalizePromotionBatch(t, fixture, http.StatusOK, promoted.ID, repeated.ID)
	if outcomes.AlreadyFinalized != 2 || countTerm(t, fixture, "student_enrollments", fixture.ids["destinationTerm"]) != 2 {
		t.Fatal("retry created duplicate enrollment")
	}
	if err := fixture.db.QueryRow(historyQuery, fixture.ids["termA"]).Scan(&historyAfter); err != nil || historyBefore != historyAfter {
		t.Fatalf("historical academic records or admission numbers changed: %v", err)
	}
	if _, err := fixture.db.Exec(`UPDATE student_promotion_decisions SET decision='REPEAT' WHERE id=$1`, promoted.ID); err == nil {
		t.Fatal("database allowed a finalized decision to change")
	}
	wrongDestination := promotionFinalizeRequest(fixture, promoted.ID)
	wrongDestination.TermID = fixture.ids["termA"]
	response := fixture.call(t, "admin", http.MethodPost, promotionFinalizePath, workflowJSON(t, wrongDestination))
	expectOwnershipStatus(t, response, http.StatusMultiStatus)
	activatePromotionSession(t, fixture, fixture.ids["sessionA"])
	request := promotionSaveRequest(fixture, "studentA", model.PromotionDecisionRepeat)
	expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodPut, promotionDecisionsPath, workflowJSON(t, request)), http.StatusConflict)
}

func TestPromotionFinalizationRejectsInvalidDestinations(t *testing.T) {
	for _, condition := range []string{"inactive destination", "wrong session", "wrong intended date", "pending", "inactive student", "missing source enrollment", "existing enrollment", "locked configuration", "not First Term"} {
		t.Run(condition, func(t *testing.T) {
			fixture := newPromotionFixture(t)
			decisionType := model.PromotionDecisionPromote
			if condition == "pending" {
				decisionType = model.PromotionDecisionPending
			}
			decision := savePromotion(t, fixture, "studentA", decisionType)
			request := promotionFinalizeRequest(fixture, decision.ID)
			if condition != "inactive destination" {
				activatePromotionSession(t, fixture, fixture.ids["destinationSession"])
			}
			var err error
			switch condition {
			case "wrong session":
				request.AcademicSessionID = fixture.ids["sessionA"]
			case "wrong intended date":
				_, err = fixture.db.Exec(`UPDATE academic_sessions SET start_date='2026-12-31' WHERE id=$1`, request.AcademicSessionID)
			case "inactive student":
				_, err = fixture.db.Exec(`UPDATE users SET is_active=false WHERE id=$1`, fixture.users["studentA"].ID)
			case "missing source enrollment":
				_, err = fixture.db.Exec(`DELETE FROM student_enrollments WHERE student_id=$1 AND term_id=$2`, fixture.users["studentA"].ID, fixture.ids["termA"])
			case "existing enrollment":
				_, err = fixture.db.Exec(`INSERT INTO student_enrollments(student_id,class_id,academic_session_id,term_id) VALUES($1,$2,$3,$4)`, fixture.users["studentA"].ID, fixture.ids["classB"], request.AcademicSessionID, request.TermID)
			case "locked configuration":
				if _, err := fixture.db.Exec(`INSERT INTO result_batches(class_id,academic_session_id,term_id,subject_id)
 VALUES($1,$2,$3,$4)`, fixture.ids["emptyClass"], request.AcademicSessionID, request.TermID, fixture.ids["math"]); err != nil {
					t.Fatal(err)
				}
				_, err = fixture.db.Exec(`INSERT INTO result_batch_events(class_id,academic_session_id,term_id,subject_id,action,actor_id)
 VALUES($1,$2,$3,$4,'submitted',$5)`, fixture.ids["emptyClass"], request.AcademicSessionID, request.TermID, fixture.ids["math"], fixture.users["admin"].ID)
			case "not First Term":
				_, err = fixture.db.Exec(`UPDATE terms SET name='SECOND_TERM' WHERE id=$1`, request.TermID)
			}
			if err != nil {
				t.Fatal(err)
			}
			response := fixture.call(t, "admin", http.MethodPost, promotionFinalizePath, workflowJSON(t, request))
			expectOwnershipStatus(t, response, http.StatusMultiStatus)
			var outcomes model.FinalizeStudentPromotionsResponse
			if err := json.Unmarshal(response.Body.Bytes(), &outcomes); err != nil || outcomes.Failed != 1 || outcomes.Succeeded != 0 {
				t.Fatalf("invalid destination accepted: %s %v", response.Body, err)
			}
			var status string
			if err := fixture.db.QueryRow(`SELECT status FROM student_promotion_decisions WHERE id=$1`, decision.ID).Scan(&status); err != nil || status != model.PromotionStatusSaved {
				t.Fatalf("failed decision was finalized: %s %v", status, err)
			}
			if condition == "locked configuration" {
				if countTerm(t, fixture, "student_enrollments", request.TermID) != 0 {
					t.Fatal("locked destination received an enrollment")
				}
				if !strings.Contains(outcomes.Results[0].Reason, "locked") {
					t.Fatalf("expected destination lock rejection: %+v", outcomes)
				}
			}
		})
	}
}

func TestPromotionPartialFailureAndConcurrentRetry(t *testing.T) {
	fixture := newPromotionFixture(t)
	promoted := savePromotion(t, fixture, "studentA", model.PromotionDecisionPromote)
	pending := savePromotion(t, fixture, "studentB", model.PromotionDecisionPending)
	activatePromotionSession(t, fixture, fixture.ids["destinationSession"])
	outcomes := finalizePromotionBatch(t, fixture, http.StatusMultiStatus, pending.ID, promoted.ID, uuid.New())
	if outcomes.Succeeded != 1 || outcomes.Failed != 2 || countTerm(t, fixture, "student_enrollments", fixture.ids["destinationTerm"]) != 1 {
		t.Fatalf("partial failure lost a successful enrollment: %+v", outcomes)
	}
	// Use a new repeating decision for two simultaneous finalization attempts.
	activatePromotionSession(t, fixture, fixture.ids["sessionA"])
	repeated := savePromotion(t, fixture, "studentB", model.PromotionDecisionRepeat)
	activatePromotionSession(t, fixture, fixture.ids["destinationSession"])
	request := promotionFinalizeRequest(fixture, repeated.ID)
	start := make(chan struct{})
	errorsFound := make(chan error, 2)
	responses := make(chan *model.FinalizeStudentPromotionsResponse, 2)
	var workers sync.WaitGroup
	for worker := 0; worker < 2; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			response, err := fixture.results.FinalizePromotions(context.Background(), fixture.users["admin"].ID, request)
			errorsFound <- err
			responses <- response
		}()
	}
	close(start)
	workers.Wait()
	close(errorsFound)
	close(responses)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	var succeeded, alreadyFinalized int
	for response := range responses {
		succeeded += response.Succeeded
		alreadyFinalized += response.AlreadyFinalized
		if response.Failed != 0 {
			t.Fatalf("concurrent finalization failed: %+v", response)
		}
	}
	if succeeded != 1 || alreadyFinalized != 1 || countTerm(t, fixture, "student_enrollments", fixture.ids["destinationTerm"]) != 2 {
		t.Fatal("concurrent requests did not finalize exactly once")
	}
}

func TestPromotionFinalizationRollback(t *testing.T) {
	for _, failure := range []string{"status update", "audit insert"} {
		t.Run(failure, func(t *testing.T) {
			fixture := newPromotionFixture(t)
			decision := savePromotion(t, fixture, "studentA", model.PromotionDecisionPromote)
			activatePromotionSession(t, fixture, fixture.ids["destinationSession"])
			query := `CREATE FUNCTION reject_promotion_write() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN RAISE EXCEPTION 'private database failure'; END $$; `
			if failure == "status update" {
				query += `CREATE TRIGGER reject_promotion_write BEFORE UPDATE ON student_promotion_decisions FOR EACH ROW EXECUTE FUNCTION reject_promotion_write()`
			} else {
				query += `CREATE TRIGGER reject_promotion_write BEFORE INSERT ON term_configuration_events FOR EACH ROW EXECUTE FUNCTION reject_promotion_write()`
			}
			if _, err := fixture.db.Exec(query); err != nil {
				t.Fatal(err)
			}
			outcomes := finalizePromotionBatch(t, fixture, http.StatusMultiStatus, decision.ID)
			if outcomes.Failed != 1 || countTerm(t, fixture, "student_enrollments", fixture.ids["destinationTerm"]) != 0 {
				t.Fatal("failed transaction retained its enrollment")
			}
			if strings.Contains(outcomes.Results[0].Reason, "private database failure") {
				t.Fatal("database diagnostic leaked")
			}
			var status string
			if err := fixture.db.QueryRow(`SELECT status FROM student_promotion_decisions WHERE id=$1`, decision.ID).Scan(&status); err != nil || status != model.PromotionStatusSaved {
				t.Fatalf("failed transaction retained finalization: %s %v", status, err)
			}
		})
	}
}
