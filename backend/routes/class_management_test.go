package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/handler"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const classManagementPath = "/api/v1/admin/classes"

// Invalid input must be rejected before a repository or database is needed.
func TestClassManagementInvalidRequests(t *testing.T) {
	classHandler := handler.NewClassHandler(service.NewClassService(nil))
	for _, testCase := range []struct {
		name, classID, body string
	}{
		{"invalid id", "invalid", `{"name":"JSS 2A","level":"JSS 2"}`},
		{"empty id", uuid.Nil.String(), `{"name":"JSS 2A","level":"JSS 2"}`},
		{"missing name", uuid.NewString(), `{"level":"JSS 2"}`},
		{"blank name", uuid.NewString(), `{"name":"  ","level":"JSS 2"}`},
		{"missing level", uuid.NewString(), `{"name":"JSS 2A"}`},
		{"blank level", uuid.NewString(), `{"name":"JSS 2A","level":"  "}`},
		{"null name", uuid.NewString(), `{"name":null,"level":"JSS 2"}`},
		{"null level", uuid.NewString(), `{"name":"JSS 2A","level":null}`},
		{"unknown field", uuid.NewString(), `{"name":"JSS 2A","level":"JSS 2","is_active":true}`},
		{"wrong type", uuid.NewString(), `{"name":123,"level":"JSS 2"}`},
		{"malformed JSON", uuid.NewString(), `{`},
		{"multiple objects", uuid.NewString(), `{"name":"JSS 2A","level":"JSS 2"} {}`},
		{"array body", uuid.NewString(), `[]`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPatch, classManagementPath+"/"+testCase.classID, strings.NewReader(testCase.body))
			request.SetPathValue("id", testCase.classID)
			response := httptest.NewRecorder()
			classHandler.Update(response, request)
			expectOwnershipStatus(t, response, http.StatusBadRequest)
		})
	}
	for _, classID := range []string{"invalid", uuid.Nil.String()} {
		request := httptest.NewRequest(http.MethodDelete, classManagementPath+"/"+classID, nil)
		request.SetPathValue("id", classID)
		response := httptest.NewRecorder()
		classHandler.Delete(response, request)
		expectOwnershipStatus(t, response, http.StatusBadRequest)
	}
}

func TestClassManagementRoutesRequireAuthentication(t *testing.T) {
	classHandler := handler.NewClassHandler(nil)
	router := SetupRoutes(handler.NewUserHandler(nil), nil, classHandler, nil, nil, nil, nil, nil, nil, ownershipSecret)
	path := classManagementPath + "/" + uuid.NewString()
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		expectOwnershipStatus(t, response, http.StatusUnauthorized)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, path, nil))
	expectOwnershipStatus(t, response, http.StatusMethodNotAllowed)
	if response.Header().Get("Allow") != "DELETE, PATCH" {
		t.Fatalf("unexpected allowed methods: %s", response.Header().Get("Allow"))
	}
}

func TestClassManagementAuthorization(t *testing.T) {
	fixture := newPromotionFixture(t)
	before := academicSessionDeletionSnapshot(t, fixture.db)
	for _, actor := range []string{"", "teacherA", "studentA", "inactive"} {
		expectedStatus := http.StatusForbidden
		if actor == "" {
			expectedStatus = http.StatusUnauthorized
		}
		for _, method := range []string{http.MethodPatch, http.MethodDelete} {
			response := fixture.call(t, actor, method, classManagementPath+"/"+fixture.ids["emptyClass"].String(), `{"name":"JSS 2A","level":"JSS 2"}`)
			expectOwnershipStatus(t, response, expectedStatus)
		}
		expectOwnershipStatus(t, fixture.call(t, actor, http.MethodPost, classManagementPath, `{"name":"JSS 2A","level":"JSS 2"}`), expectedStatus)
	}
	if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
		t.Fatal("unauthorized class management changed stored records")
	}
}

// Reuse the fixture with migrations through 018 so promotion references are
// present as well as enrollment, assignments, results and attendance constraints.
func TestClassManagementCreateUpdateAndList(t *testing.T) {
	fixture := newPromotionFixture(t)
	response := fixture.call(t, "admin", http.MethodPost, classManagementPath, `{"name":"  JSS 2A  ","level":"  JSS 2  "}`)
	expectOwnershipStatus(t, response, http.StatusCreated)
	var createdClass model.Class
	if err := json.Unmarshal(response.Body.Bytes(), &createdClass); err != nil {
		t.Fatal(err)
	}
	if createdClass.ID == uuid.Nil || createdClass.Name != "JSS 2A" || createdClass.Level != "JSS 2" || createdClass.CreatedAt.IsZero() || createdClass.UpdatedAt.IsZero() {
		t.Fatalf("unexpected created class: %+v", createdClass)
	}

	path := classManagementPath + "/" + createdClass.ID.String()
	response = fixture.call(t, "admin", http.MethodPatch, path, `{"name":"  JSS 3A  ","level":"  JSS 3  "}`)
	expectOwnershipStatus(t, response, http.StatusOK)
	var updatedClass model.Class
	if err := json.Unmarshal(response.Body.Bytes(), &updatedClass); err != nil {
		t.Fatal(err)
	}
	if updatedClass.ID != createdClass.ID || updatedClass.Name != "JSS 3A" || updatedClass.Level != "JSS 3" || !updatedClass.CreatedAt.Equal(createdClass.CreatedAt) || !updatedClass.UpdatedAt.After(createdClass.UpdatedAt) {
		t.Fatalf("unexpected updated class: %+v", updatedClass)
	}
	storedClass, err := repository.NewClassRepository(fixture.db).GetByID(t.Context(), createdClass.ID)
	if err != nil || storedClass == nil || storedClass.ID != updatedClass.ID || storedClass.Name != updatedClass.Name || storedClass.Level != updatedClass.Level || !storedClass.CreatedAt.Equal(updatedClass.CreatedAt) || !storedClass.UpdatedAt.Equal(updatedClass.UpdatedAt) {
		t.Fatalf("updated response differs from stored class: %+v %v", storedClass, err)
	}

	response = fixture.call(t, "admin", http.MethodGet, classManagementPath, "")
	expectOwnershipStatus(t, response, http.StatusOK)
	var classes []model.Class
	if err := json.Unmarshal(response.Body.Bytes(), &classes); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, class := range classes {
		if class.ID == updatedClass.ID {
			found = true
			if class != updatedClass {
				t.Fatal("class list does not reflect the update")
			}
		}
	}
	if !found {
		t.Fatal("updated class missing from list")
	}
}

func TestClassManagementDuplicateNamesAndMissingClasses(t *testing.T) {
	fixture := newPromotionFixture(t)
	before := academicSessionDeletionSnapshot(t, fixture.db)
	for _, request := range []struct{ method, path string }{
		{http.MethodPost, classManagementPath},
		{http.MethodPatch, classManagementPath + "/" + fixture.ids["emptyClass"].String()},
	} {
		response := fixture.call(t, "admin", request.method, request.path, `{"name":"classA","level":"JSS 2"}`)
		expectOwnershipStatus(t, response, http.StatusConflict)
		if !strings.Contains(response.Body.String(), "a class with this name already exists") {
			classService := service.NewClassService(repository.NewClassRepository(fixture.db))
			var serviceError error
			if request.method == http.MethodPost {
				_, serviceError = classService.Create(t.Context(), "classA", "JSS 2")
			} else {
				_, serviceError = classService.Update(t.Context(), fixture.ids["emptyClass"], "classA", "JSS 2")
			}
			logClassManagementError(t, "duplicate class service", serviceError)
			t.Fatal("duplicate name did not return a descriptive conflict", response.Body)
		}
	}
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		expectOwnershipStatus(t, fixture.call(t, "admin", method, classManagementPath+"/"+uuid.NewString(), `{"name":"JSS 2A","level":"JSS 2"}`), http.StatusNotFound)
	}
	if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
		t.Fatal("failed class management changed stored records")
	}
}

func TestClassManagementDeleteUnreferencedClass(t *testing.T) {
	fixture := newPromotionFixture(t)
	classID := fixture.ids["emptyClass"]
	// Compare academic records independently of the expected class deletion.
	before := classManagementRelatedRecords(t, fixture)
	response := fixture.call(t, "admin", http.MethodDelete, classManagementPath+"/"+classID.String(), "")
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("expected empty 204 response: %d %s", response.Code, response.Body)
	}
	var remaining int
	if err := fixture.db.QueryRow(`SELECT count(*) FROM classes WHERE id=$1`, classID).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("class was not deleted: %d %v", remaining, err)
	}
	if after := classManagementRelatedRecords(t, fixture); after != before {
		t.Fatal("deleting an unused class changed academic records")
	}
	expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, classManagementPath+"/"+classID.String(), ""), http.StatusNotFound)
}

// Class names and levels are shared labels. Updating them must not rewrite
// historical placements, scores, attendance, assignments or promotion decisions.
func TestClassManagementUpdateReferencedClassPreservesRecords(t *testing.T) {
	fixture := newPromotionFixture(t)
	expectOwnershipStatus(t, fixture.draft(t, "teacherA", "studentA", "math", 30, 50), http.StatusCreated)
	savePromotion(t, fixture, "studentA", model.PromotionDecisionPromote)
	if _, err := fixture.db.Exec(`INSERT INTO attendance(student_id,class_id,academic_session_id,term_id,date,status,recorded_by) VALUES($1,$2,$3,$4,'2026-02-02','PRESENT',$5)`, fixture.users["studentA"].ID, fixture.ids["classA"], fixture.ids["sessionA"], fixture.ids["termA"], fixture.users["teacherA"].ID); err != nil {
		t.Fatal(err)
	}
	before := classManagementRelatedRecords(t, fixture)
	response := fixture.call(t, "admin", http.MethodPatch, classManagementPath+"/"+fixture.ids["classA"].String(), `{"name":"JSS 1A renamed","level":"JSS 1"}`)
	expectOwnershipStatus(t, response, http.StatusOK)
	if after := classManagementRelatedRecords(t, fixture); after != before {
		t.Fatal("updating a class changed academic records")
	}
}

func classManagementRelatedRecords(t *testing.T, fixture *ownershipFixture) string {
	t.Helper()
	var records strings.Builder
	for _, line := range strings.Split(academicSessionDeletionSnapshot(t, fixture.db), "\n") {
		if !strings.HasPrefix(line, "classes:") {
			records.WriteString(line + "\n")
		}
	}
	return records.String()
}

func TestClassManagementDeleteReferencedClass(t *testing.T) {
	for _, referenceType := range []string{
		"enrollment", "class teacher", "subject offering", "subject teacher", "draft result",
		"attendance", "result batch", "publication", "promotion source", "promotion target",
	} {
		t.Run(referenceType, func(t *testing.T) {
			fixture := newPromotionFixture(t)
			classID := fixture.ids["emptyClass"]
			sessionID, termID := fixture.ids["sessionA"], fixture.ids["termA"]
			studentID, teacherID := fixture.users["studentA"].ID, fixture.users["teacherA"].ID
			subjectID, adminID := fixture.ids["math"], fixture.users["admin"].ID
			executeSetup := func(query string, arguments ...any) {
				t.Helper()
				if _, err := fixture.db.Exec(query, arguments...); err != nil {
					t.Fatal(err)
				}
			}
			switch referenceType {
			case "enrollment":
				// Use the destination term so the student is not already enrolled.
				executeSetup(`INSERT INTO student_enrollments(student_id,class_id,academic_session_id,term_id) VALUES($1,$2,$3,$4)`, studentID, classID, fixture.ids["destinationSession"], fixture.ids["destinationTerm"])
			case "class teacher":
				executeSetup(`INSERT INTO class_teacher_assignments(teacher_id,class_id,academic_session_id,term_id) VALUES($1,$2,$3,$4)`, fixture.users["unassigned"].ID, classID, sessionID, termID)
			case "subject offering", "subject teacher":
				executeSetup(`INSERT INTO class_subject_offerings(class_id,academic_session_id,term_id,subject_id,created_by) VALUES($1,$2,$3,$4,$5)`, classID, sessionID, termID, subjectID, adminID)
				if referenceType == "subject teacher" {
					executeSetup(`INSERT INTO teacher_subject_assignments(teacher_id,class_id,academic_session_id,term_id,subject_id) VALUES($1,$2,$3,$4,$5)`, teacherID, classID, sessionID, termID, subjectID)
				}
			case "result batch":
				executeSetup(`INSERT INTO result_batches(class_id,academic_session_id,term_id,subject_id) VALUES($1,$2,$3,$4)`, classID, sessionID, termID, subjectID)
			case "publication":
				executeSetup(`INSERT INTO class_term_publications(class_id,academic_session_id,term_id,published_by) VALUES($1,$2,$3,$4)`, classID, sessionID, termID, adminID)
			case "promotion source":
				executeSetup(`INSERT INTO student_promotion_decisions(student_id,source_class_id,source_academic_session_id,source_term_id,decision,target_session_start_date,decided_by) VALUES($1,$2,$3,$4,'PENDING','2027-01-01',$5)`, studentID, classID, sessionID, termID, adminID)
			case "promotion target":
				savePromotion(t, fixture, "studentA", model.PromotionDecisionPromote)
			case "draft result":
				classID = fixture.ids["classA"]
				expectOwnershipStatus(t, fixture.draft(t, "teacherA", "studentA", "math", 30, 50), http.StatusCreated)
			case "attendance":
				classID = fixture.ids["classA"]
				executeSetup(`INSERT INTO attendance(student_id,class_id,academic_session_id,term_id,date,status,recorded_by) VALUES($1,$2,$3,$4,'2026-02-02','PRESENT',$5)`, studentID, classID, sessionID, termID, teacherID)
			}
			before := academicSessionDeletionSnapshot(t, fixture.db)
			response := fixture.call(t, "admin", http.MethodDelete, classManagementPath+"/"+classID.String(), "")
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "class cannot be deleted") {
				classRepository := repository.NewClassRepository(fixture.db)
				logClassManagementError(t, "referenced class repository", classRepository.Delete(t.Context(), classID))
				logClassManagementError(t, "referenced class service", service.NewClassService(classRepository).Delete(t.Context(), classID))
			}
			if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
				t.Fatal("rejected class deletion changed academic records or configuration")
			}
			expectOwnershipStatus(t, response, http.StatusConflict)
			if !strings.Contains(response.Body.String(), "class cannot be deleted") {
				t.Fatal("referenced class did not return a descriptive conflict", response.Body)
			}
		})
	}
}

// Report only types, SQLSTATE and domain classification, never raw database
// messages, connection strings or credentials.
func logClassManagementError(t *testing.T, operation string, operationError error) {
	t.Helper()
	t.Logf("%s: error_type=%T conflict=%t not_found=%t", operation, operationError,
		errors.Is(operationError, service.ErrConflict), errors.Is(operationError, service.ErrNotFound))
	var databaseError *pq.Error
	if errors.As(operationError, &databaseError) {
		t.Logf("%s: PostgreSQL SQLSTATE=%s", operation, databaseError.Code)
	}
	for cause := errors.Unwrap(operationError); cause != nil; cause = errors.Unwrap(cause) {
		t.Logf("%s: underlying_error_type=%T", operation, cause)
	}
}
