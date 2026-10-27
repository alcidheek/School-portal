package routes

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alcidheek/school-portal/internal/handler"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const academicSessionDeletionPath = "/api/v1/admin/academic-sessions/"

// These checks run even without a PostgreSQL test database.
func TestAcademicSessionDeletionRoutingAndInvalidID(t *testing.T) {
	academicHandler := handler.NewAcademicSessionHandler(service.NewAcademicSessionService(nil, nil))
	router := SetupRoutes(handler.NewUserHandler(nil), academicHandler, nil, nil, nil, nil, nil, nil, nil, ownershipSecret)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, academicSessionDeletionPath+uuid.NewString(), nil))
	expectOwnershipStatus(t, response, http.StatusUnauthorized)

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, academicSessionDeletionPath+uuid.NewString(), nil))
	expectOwnershipStatus(t, response, http.StatusMethodNotAllowed)
	if response.Header().Get("Allow") != http.MethodDelete {
		t.Fatalf("unexpected allowed methods: %s", response.Header().Get("Allow"))
	}

	for _, sessionID := range []string{"invalid", uuid.Nil.String()} {
		request := httptest.NewRequest(http.MethodDelete, academicSessionDeletionPath+sessionID, nil)
		request.SetPathValue("id", sessionID)
		response := httptest.NewRecorder()
		academicHandler.Delete(response, request)
		expectOwnershipStatus(t, response, http.StatusBadRequest)
	}
}

func newAcademicSessionDeletionFixture(t *testing.T) (*ownershipFixture, *service.AcademicSessionService, uuid.UUID, uuid.UUID) {
	t.Helper()
	fixture := newOwnershipFixture(t)
	for _, migrationName := range []string{"017_student_promotion_decisions.sql", "018_allow_saved_promotion_decision_deletion.sql"} {
		migration, err := os.ReadFile("../migrations/" + migrationName)
		if err != nil {
			t.Fatal(err)
		}
		transaction, err := fixture.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := transaction.Exec(string(migration)); err != nil {
			transaction.Rollback()
			t.Fatal(err)
		}
		if err := transaction.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	academicService := service.NewAcademicSessionService(repository.NewAcademicSessionRepository(fixture.db), repository.NewTermRepository(fixture.db))
	session, err := academicService.Create(context.Background(), "Deletion candidate", "2027-01-01", "2027-12-31", false, "FIRST_TERM", "2027-01-01", "2027-04-01")
	if err != nil {
		t.Fatal(err)
	}
	var termID uuid.UUID
	if err := fixture.db.QueryRow(`SELECT id FROM terms WHERE academic_session_id=$1`, session.ID).Scan(&termID); err != nil {
		t.Fatal(err)
	}
	return fixture, academicService, session.ID, termID
}

func executeAcademicSessionDeletionSetup(t *testing.T, fixture *ownershipFixture, query string, arguments ...any) {
	t.Helper()
	if _, err := fixture.db.Exec(query, arguments...); err != nil {
		t.Fatal(err)
	}
}

func addAcademicSessionDeletionConfiguration(t *testing.T, fixture *ownershipFixture, sessionID, termID uuid.UUID) {
	t.Helper()
	executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO class_subject_offerings(class_id,academic_session_id,term_id,subject_id,created_by)
		VALUES($1,$2,$3,$4,$5)`, fixture.ids["classA"], sessionID, termID, fixture.ids["math"], fixture.users["admin"].ID)
	executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO class_teacher_assignments(teacher_id,class_id,academic_session_id,term_id)
		VALUES($1,$2,$3,$4)`, fixture.users["teacherA"].ID, fixture.ids["classA"], sessionID, termID)
	executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO teacher_subject_assignments(teacher_id,class_id,academic_session_id,term_id,subject_id)
		VALUES($1,$2,$3,$4,$5)`, fixture.users["teacherA"].ID, fixture.ids["classA"], sessionID, termID, fixture.ids["math"])
	executeAcademicSessionDeletionSetup(t, fixture, `UPDATE academic_sessions SET last_active_term_id=$2 WHERE id=$1`, sessionID, termID)
}

// Compare complete rows, including timestamps and history, after rejected or
// failed deletion. Sorting JSON text makes tables without an id deterministic.
func academicSessionDeletionSnapshot(t *testing.T, database *sql.DB) string {
	t.Helper()
	var snapshot strings.Builder
	for _, tableName := range []string{
		"academic_sessions", "terms", "student_enrollments", "results", "attendance",
		"class_teacher_assignments", "teacher_subject_assignments", "class_subject_offerings",
		"result_batches", "result_batch_events", "class_term_publications",
		"term_configuration_events", "term_initializations", "student_promotion_decisions",
		"users", "classes", "subjects",
	} {
		var records string
		query := `SELECT COALESCE(jsonb_agg(record ORDER BY record::text), '[]'::jsonb)::text
			FROM (SELECT to_jsonb(records) AS record FROM ` + pq.QuoteIdentifier(tableName) + ` records) snapshot`
		if err := database.QueryRow(query).Scan(&records); err != nil {
			t.Fatal(err)
		}
		snapshot.WriteString(tableName + ":" + records + "\n")
	}
	return snapshot.String()
}

func TestAcademicSessionDeletionAuthorizationAndMissingSession(t *testing.T) {
	fixture, _, sessionID, _ := newAcademicSessionDeletionFixture(t)
	before := academicSessionDeletionSnapshot(t, fixture.db)
	for _, actor := range []string{"", "teacherA", "studentA", "inactive"} {
		expectedStatus := http.StatusForbidden
		if actor == "" {
			expectedStatus = http.StatusUnauthorized
		}
		expectOwnershipStatus(t, fixture.call(t, actor, http.MethodDelete, academicSessionDeletionPath+sessionID.String(), ""), expectedStatus)
	}
	expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, academicSessionDeletionPath+uuid.NewString(), ""), http.StatusNotFound)
	if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
		t.Fatal("unauthorized or missing-session deletion changed stored records")
	}
}

func TestAcademicSessionDeletionUnusedSession(t *testing.T) {
	t.Run("no terms", func(t *testing.T) {
		fixture, _, _, _ := newAcademicSessionDeletionFixture(t)
		response := fixture.call(t, "admin", http.MethodDelete, academicSessionDeletionPath+fixture.ids["emptySession"].String(), "")
		if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
			t.Fatalf("expected empty 204 response: %d %s", response.Code, response.Body)
		}
		expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, academicSessionDeletionPath+fixture.ids["emptySession"].String(), ""), http.StatusNotFound)
	})

	for _, withConfiguration := range []bool{false, true} {
		name := "terms only"
		if withConfiguration {
			name = "safe configuration and remembered term"
		}
		t.Run(name, func(t *testing.T) {
			fixture, _, sessionID, termID := newAcademicSessionDeletionFixture(t)
			if withConfiguration {
				addAcademicSessionDeletionConfiguration(t, fixture, sessionID, termID)
			}
			response := fixture.call(t, "admin", http.MethodDelete, academicSessionDeletionPath+sessionID.String(), "")
			if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
				t.Fatalf("expected empty 204 response: %d %s", response.Code, response.Body)
			}
			for _, tableName := range []string{"academic_sessions", "terms", "class_subject_offerings", "class_teacher_assignments", "teacher_subject_assignments"} {
				columnName := "academic_session_id"
				if tableName == "academic_sessions" {
					columnName = "id"
				}
				var remaining int
				if err := fixture.db.QueryRow(`SELECT count(*) FROM `+pq.QuoteIdentifier(tableName)+` WHERE `+columnName+`=$1`, sessionID).Scan(&remaining); err != nil || remaining != 0 {
					t.Fatalf("session rows remain in %s: %d %v", tableName, remaining, err)
				}
			}
			// The existing populated session must remain untouched by cleanup.
			if countTerm(t, fixture, "student_enrollments", fixture.ids["termA"]) != 2 || countTerm(t, fixture, "teacher_subject_assignments", fixture.ids["termA"]) != 3 {
				t.Fatal("deletion changed unrelated session records")
			}
			expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, academicSessionDeletionPath+sessionID.String(), ""), http.StatusNotFound)
		})
	}
}

func TestAcademicSessionDeletionUnboundPromotionDate(t *testing.T) {
	fixture, _, sessionID, _ := newAcademicSessionDeletionFixture(t)
	decisionID := uuid.New()
	executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO student_promotion_decisions(id,student_id,source_class_id,source_academic_session_id,source_term_id,decision,target_session_start_date,decided_by)
		VALUES($1,$2,$3,$4,$5,'PENDING','2027-01-01',$6)`, decisionID, fixture.users["studentA"].ID, fixture.ids["classA"], fixture.ids["sessionA"], fixture.ids["termA"], fixture.users["admin"].ID)
	response := fixture.call(t, "admin", http.MethodDelete, academicSessionDeletionPath+sessionID.String(), "")
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("unbound intended date blocked deletion: %d %s", response.Code, response.Body)
	}
	var decisionRemains bool
	if err := fixture.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM student_promotion_decisions WHERE id=$1 AND target_academic_session_id IS NULL AND target_session_start_date='2027-01-01')`, decisionID).Scan(&decisionRemains); err != nil || !decisionRemains {
		t.Fatalf("deletion changed unbound promotion intent: %v", err)
	}
}

func TestAcademicSessionDeletionActiveSessionOrTerm(t *testing.T) {
	for _, activeSession := range []bool{true, false} {
		name := "active term in inactive session"
		if activeSession {
			name = "active session"
		}
		t.Run(name, func(t *testing.T) {
			fixture, academicService, sessionID, termID := newAcademicSessionDeletionFixture(t)
			if activeSession {
				if err := academicService.Activate(context.Background(), sessionID); err != nil {
					t.Fatal(err)
				}
			} else {
				// Existing schema permits this inconsistent state; deletion fails closed.
				executeAcademicSessionDeletionSetup(t, fixture, `UPDATE terms SET is_active=true WHERE id=$1`, termID)
			}
			before := academicSessionDeletionSnapshot(t, fixture.db)
			expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, academicSessionDeletionPath+sessionID.String(), ""), http.StatusConflict)
			if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
				t.Fatal("active-session rejection changed stored records")
			}
		})
	}
}

func TestAcademicSessionDeletionProtectedRecords(t *testing.T) {
	for _, recordType := range []string{
		"enrollment", "draft results", "attendance", "empty draft batch", "batch event", "publication",
		"configuration history", "initialization source", "initialization target",
		"saved promotion source", "saved promotion target", "finalized promotion target",
	} {
		t.Run(recordType, func(t *testing.T) {
			fixture, _, sessionID, termID := newAcademicSessionDeletionFixture(t)
			addAcademicSessionDeletionConfiguration(t, fixture, sessionID, termID)
			studentID, classID := fixture.users["studentA"].ID, fixture.ids["classA"]
			adminID, subjectID := fixture.users["admin"].ID, fixture.ids["math"]
			if recordType == "enrollment" || recordType == "draft results" || recordType == "attendance" || recordType == "finalized promotion target" {
				executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO student_enrollments(student_id,class_id,academic_session_id,term_id) VALUES($1,$2,$3,$4)`, studentID, classID, sessionID, termID)
			}
			if recordType == "empty draft batch" || recordType == "draft results" || recordType == "batch event" {
				executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO result_batches(class_id,academic_session_id,term_id,subject_id) VALUES($1,$2,$3,$4)`, classID, sessionID, termID, subjectID)
			}
			switch recordType {
			case "draft results":
				executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO results(student_id,subject_id,class_id,academic_session_id,term_id,ca_score,exam_score,total_score,grade,remark) VALUES($1,$2,$3,$4,$5,30,50,80,'A','Excellent')`, studentID, subjectID, classID, sessionID, termID)
			case "attendance":
				executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO attendance(student_id,class_id,academic_session_id,term_id,date,status,recorded_by) VALUES($1,$2,$3,$4,'2027-02-01','PRESENT',$5)`, studentID, classID, sessionID, termID, fixture.users["teacherA"].ID)
			case "batch event":
				executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO result_batch_events(class_id,academic_session_id,term_id,subject_id,action,actor_id) VALUES($1,$2,$3,$4,'submitted',$5)`, classID, sessionID, termID, subjectID, adminID)
			case "publication":
				executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO class_term_publications(class_id,academic_session_id,term_id,published_by) VALUES($1,$2,$3,$4)`, classID, sessionID, termID, adminID)
			case "configuration history":
				executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO term_configuration_events(academic_session_id,term_id,actor_id,action) VALUES($1,$2,$3,'offerings.add')`, sessionID, termID, adminID)
			case "initialization source", "initialization target":
				sourceTermID, targetTermID := termID, fixture.ids["termA"]
				if recordType == "initialization target" {
					sourceTermID, targetTermID = targetTermID, sourceTermID
				}
				executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO term_initializations(source_term_id,target_term_id,category,initialized_by) VALUES($1,$2,'offerings',$3)`, sourceTermID, targetTermID, adminID)
			case "saved promotion source":
				executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO student_promotion_decisions(student_id,source_class_id,source_academic_session_id,source_term_id,decision,target_session_start_date,decided_by) VALUES($1,$2,$3,$4,'PENDING','2028-01-01',$5)`, studentID, classID, sessionID, termID, adminID)
			case "saved promotion target", "finalized promotion target":
				decisionID := uuid.New()
				executeAcademicSessionDeletionSetup(t, fixture, `INSERT INTO student_promotion_decisions(id,student_id,source_class_id,source_academic_session_id,source_term_id,decision,target_class_id,target_academic_session_id,target_session_start_date,decided_by) VALUES($1,$2,$3,$4,$5,'REPEAT',$3,$6,'2027-01-01',$7)`, decisionID, studentID, classID, fixture.ids["sessionA"], fixture.ids["termA"], sessionID, adminID)
				if recordType == "finalized promotion target" {
					activatePromotionSession(t, fixture, sessionID)
					executeAcademicSessionDeletionSetup(t, fixture, `UPDATE student_promotion_decisions SET status='FINALIZED',finalized_by=$2,finalized_term_id=$3,finalized_at=NOW() WHERE id=$1`, decisionID, adminID, termID)
					activatePromotionSession(t, fixture, fixture.ids["sessionB"])
				}
			}
			before := academicSessionDeletionSnapshot(t, fixture.db)
			expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, academicSessionDeletionPath+sessionID.String(), ""), http.StatusConflict)
			if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
				t.Fatal("protected-record rejection changed stored records")
			}
		})
	}
}

func TestAcademicSessionDeletionFailureRollsBack(t *testing.T) {
	for _, deferredFailure := range []bool{false, true} {
		name := "statement failure"
		if deferredFailure {
			name = "commit constraint failure"
		}
		t.Run(name, func(t *testing.T) {
			fixture, _, sessionID, termID := newAcademicSessionDeletionFixture(t)
			addAcademicSessionDeletionConfiguration(t, fixture, sessionID, termID)
			// Add a test-only failure without altering any existing guard or FK.
			query := `CREATE FUNCTION reject_test_session_deletion() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'private deletion failure'; END $$;
				CREATE TRIGGER reject_test_session_deletion BEFORE DELETE ON academic_sessions
				FOR EACH ROW EXECUTE FUNCTION reject_test_session_deletion()`
			expectedStatus := http.StatusInternalServerError
			if deferredFailure {
				query = `CREATE FUNCTION reject_test_session_deletion() RETURNS trigger LANGUAGE plpgsql AS $$
					BEGIN RAISE EXCEPTION USING ERRCODE='23503', MESSAGE='private commit failure'; END $$;
					CREATE CONSTRAINT TRIGGER reject_test_session_deletion AFTER DELETE ON academic_sessions
					DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_test_session_deletion()`
				expectedStatus = http.StatusConflict
			}
			executeAcademicSessionDeletionSetup(t, fixture, query)
			before := academicSessionDeletionSnapshot(t, fixture.db)
			response := fixture.call(t, "admin", http.MethodDelete, academicSessionDeletionPath+sessionID.String(), "")
			expectOwnershipStatus(t, response, expectedStatus)
			if strings.Contains(response.Body.String(), "private") {
				t.Fatal("database failure details exposed")
			}
			if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
				t.Fatal("failed deletion did not restore configuration, terms and remembered pointer")
			}
		})
	}
}

func TestAcademicSessionDeletionConcurrentActivation(t *testing.T) {
	fixture, academicService, sessionID, termID := newAcademicSessionDeletionFixture(t)
	requestContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	activationResult, deletionResult := make(chan error, 1), make(chan error, 1)
	go func() { <-start; activationResult <- academicService.Activate(requestContext, sessionID) }()
	go func() { <-start; deletionResult <- academicService.Delete(requestContext, sessionID) }()
	close(start)
	activationError, deletionError := <-activationResult, <-deletionResult
	if activationError == nil {
		if !errors.Is(deletionError, service.ErrConflict) {
			t.Fatalf("activation won but deletion was not rejected: %v", deletionError)
		}
		var bothActive bool
		if err := fixture.db.QueryRow(`SELECT session.is_active AND term.is_active FROM academic_sessions session JOIN terms term ON term.academic_session_id=session.id WHERE session.id=$1 AND term.id=$2`, sessionID, termID).Scan(&bothActive); err != nil || !bothActive {
			t.Fatalf("activated session lost its active term: %v", err)
		}
	} else if deletionError != nil || !errors.Is(activationError, service.ErrAcademicSessionNotFound) {
		t.Fatalf("unexpected race outcome: activation=%v deletion=%v", activationError, deletionError)
	}
}

func TestAcademicSessionDeletionConcurrentEnrollment(t *testing.T) {
	fixture, academicService, sessionID, termID := newAcademicSessionDeletionFixture(t)
	requestContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	enrollmentResult, deletionResult := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		_, err := fixture.db.ExecContext(requestContext, `INSERT INTO student_enrollments(student_id,class_id,academic_session_id,term_id) VALUES($1,$2,$3,$4)`, fixture.users["studentA"].ID, fixture.ids["classA"], sessionID, termID)
		enrollmentResult <- err
	}()
	go func() { <-start; deletionResult <- academicService.Delete(requestContext, sessionID) }()
	close(start)
	enrollmentError, deletionError := <-enrollmentResult, <-deletionResult
	if enrollmentError == nil {
		if !errors.Is(deletionError, service.ErrConflict) || countTerm(t, fixture, "student_enrollments", termID) != 1 {
			t.Fatalf("committed enrollment was not protected: %v", deletionError)
		}
	} else {
		var databaseError *pq.Error
		if deletionError != nil || !errors.As(enrollmentError, &databaseError) || databaseError.Code != "23503" {
			t.Fatalf("unexpected race outcome: enrollment=%v deletion=%v", enrollmentError, deletionError)
		}
	}
}
