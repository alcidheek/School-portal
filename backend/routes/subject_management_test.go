package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/handler"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

const subjectManagementPath = "/api/v1/admin/subjects"

func TestSubjectManagementInvalidRequests(t *testing.T) {
	subjectHandler := handler.NewSubjectHandler(service.NewSubjectService(nil))
	for _, testCase := range []struct{ name, subjectID, body string }{
		{"invalid id", "invalid", `{"name":"Biology"}`},
		{"zero id", uuid.Nil.String(), `{"name":"Biology"}`},
		{"missing name", uuid.NewString(), `{}`},
		{"blank name", uuid.NewString(), `{"name":"  "}`},
		{"null name", uuid.NewString(), `{"name":null}`},
		{"unknown field", uuid.NewString(), `{"name":"Biology","level":"SSS 1"}`},
		{"wrong type", uuid.NewString(), `{"name":123}`},
		{"malformed JSON", uuid.NewString(), `{`},
		{"multiple objects", uuid.NewString(), `{"name":"Biology"} {}`},
		{"array body", uuid.NewString(), `[]`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPatch, subjectManagementPath+"/"+testCase.subjectID, strings.NewReader(testCase.body))
			request.SetPathValue("id", testCase.subjectID)
			response := httptest.NewRecorder()
			subjectHandler.Update(response, request)
			expectOwnershipStatus(t, response, http.StatusBadRequest)
		})
	}
	for _, subjectID := range []string{"invalid", uuid.Nil.String()} {
		request := httptest.NewRequest(http.MethodDelete, subjectManagementPath+"/"+subjectID, nil)
		request.SetPathValue("id", subjectID)
		response := httptest.NewRecorder()
		subjectHandler.Delete(response, request)
		expectOwnershipStatus(t, response, http.StatusBadRequest)
	}
	for _, body := range []string{`{}`, `{"name":"  "}`} {
		response := httptest.NewRecorder()
		subjectHandler.Create(response, httptest.NewRequest(http.MethodPost, subjectManagementPath, strings.NewReader(body)))
		expectOwnershipStatus(t, response, http.StatusBadRequest)
	}
}

func TestSubjectManagementRoutesRequireAuthentication(t *testing.T) {
	router := SetupRoutes(handler.NewUserHandler(nil), nil, nil, handler.NewSubjectHandler(nil), nil, nil, nil, nil, nil, ownershipSecret)
	path := subjectManagementPath + "/" + uuid.NewString()
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

func TestSubjectManagementAuthorization(t *testing.T) {
	fixture := newPromotionFixture(t)
	before := academicSessionDeletionSnapshot(t, fixture.db)
	for _, actor := range []string{"", "teacherA", "studentA", "inactive"} {
		expectedStatus := http.StatusForbidden
		if actor == "" {
			expectedStatus = http.StatusUnauthorized
		}
		for _, method := range []string{http.MethodPatch, http.MethodDelete} {
			expectOwnershipStatus(t, fixture.call(t, actor, method, subjectManagementPath+"/"+fixture.ids["math"].String(), `{"name":"Biology"}`), expectedStatus)
		}
		expectOwnershipStatus(t, fixture.call(t, actor, http.MethodPost, subjectManagementPath, `{"name":"Biology"}`), expectedStatus)
	}
	if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
		t.Fatal("unauthorized subject management changed stored records")
	}
}

func TestSubjectManagementCreateUpdateAndList(t *testing.T) {
	fixture := newPromotionFixture(t)
	response := fixture.call(t, "admin", http.MethodPost, subjectManagementPath, `{"name":"  Biology  "}`)
	expectOwnershipStatus(t, response, http.StatusCreated)
	var createdSubject model.Subject
	if err := json.Unmarshal(response.Body.Bytes(), &createdSubject); err != nil {
		t.Fatal(err)
	}
	if createdSubject.ID == uuid.Nil || createdSubject.Name != "Biology" || createdSubject.CreatedAt.IsZero() || createdSubject.UpdatedAt.IsZero() {
		t.Fatalf("unexpected created subject: %+v", createdSubject)
	}
	response = fixture.call(t, "admin", http.MethodPatch, subjectManagementPath+"/"+createdSubject.ID.String(), `{"name":"  Chemistry  "}`)
	expectOwnershipStatus(t, response, http.StatusOK)
	var updatedSubject model.Subject
	if err := json.Unmarshal(response.Body.Bytes(), &updatedSubject); err != nil {
		t.Fatal(err)
	}
	if updatedSubject.ID != createdSubject.ID || updatedSubject.Name != "Chemistry" || !updatedSubject.CreatedAt.Equal(createdSubject.CreatedAt) || !updatedSubject.UpdatedAt.After(createdSubject.UpdatedAt) {
		t.Fatalf("unexpected updated subject: %+v", updatedSubject)
	}
	storedSubject, err := repository.NewSubjectRepository(fixture.db).GetByID(t.Context(), createdSubject.ID)
	if err != nil || storedSubject == nil || storedSubject.ID != updatedSubject.ID || storedSubject.Name != updatedSubject.Name || !storedSubject.CreatedAt.Equal(updatedSubject.CreatedAt) || !storedSubject.UpdatedAt.Equal(updatedSubject.UpdatedAt) {
		t.Fatalf("updated response differs from stored subject: %+v %v", storedSubject, err)
	}
	response = fixture.call(t, "admin", http.MethodGet, subjectManagementPath, "")
	expectOwnershipStatus(t, response, http.StatusOK)
	var subjects []model.Subject
	if err := json.Unmarshal(response.Body.Bytes(), &subjects); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, subject := range subjects {
		if subject.ID == updatedSubject.ID && subject.Name == updatedSubject.Name {
			found = true
		}
	}
	if !found {
		t.Fatal("updated subject missing from list")
	}
}

func TestSubjectManagementDuplicateNamesAndMissingSubjects(t *testing.T) {
	fixture := newPromotionFixture(t)
	before := academicSessionDeletionSnapshot(t, fixture.db)
	for _, request := range []struct{ method, path string }{
		{http.MethodPost, subjectManagementPath},
		{http.MethodPatch, subjectManagementPath + "/" + fixture.ids["science"].String()},
	} {
		response := fixture.call(t, "admin", request.method, request.path, `{"name":"  math  "}`)
		expectOwnershipStatus(t, response, http.StatusConflict)
		if !strings.Contains(response.Body.String(), "a subject with this name already exists") {
			t.Fatal("duplicate name did not return a descriptive conflict", response.Body)
		}
	}
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		expectOwnershipStatus(t, fixture.call(t, "admin", method, subjectManagementPath+"/"+uuid.NewString(), `{"name":"Biology"}`), http.StatusNotFound)
	}
	if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
		t.Fatal("failed subject management changed stored records")
	}
}

func TestSubjectManagementDeleteUnreferencedSubject(t *testing.T) {
	fixture := newPromotionFixture(t)
	// Create directly so the complete before/after snapshot can match exactly.
	before := academicSessionDeletionSnapshot(t, fixture.db)
	subject := &model.Subject{ID: uuid.New(), Name: "Unused subject"}
	if err := repository.NewSubjectRepository(fixture.db).Create(t.Context(), subject); err != nil {
		t.Fatal(err)
	}
	path := subjectManagementPath + "/" + subject.ID.String()
	response := fixture.call(t, "admin", http.MethodDelete, path, "")
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("expected empty 204 response: %d %s", response.Code, response.Body)
	}
	if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
		t.Fatal("unused subject deletion changed unrelated records or failed to delete the subject")
	}
	expectOwnershipStatus(t, fixture.call(t, "admin", http.MethodDelete, path, ""), http.StatusNotFound)
}

func TestSubjectManagementReferencedSubjectPreservesRecords(t *testing.T) {
	for _, referenceType := range []string{"offering", "teacher assignment", "draft result", "empty result batch", "submission history"} {
		t.Run(referenceType, func(t *testing.T) {
			fixture := newPromotionFixture(t)
			subjectID := uuid.New()
			executeSetup := func(query string, arguments ...any) {
				t.Helper()
				if _, err := fixture.db.Exec(query, arguments...); err != nil {
					t.Fatal(err)
				}
			}
			executeSetup(`INSERT INTO subjects(id,name) VALUES($1,'Referenced subject')`, subjectID)
			classID, sessionID, termID := fixture.ids["classA"], fixture.ids["sessionA"], fixture.ids["termA"]
			adminID := fixture.users["admin"].ID
			if referenceType == "offering" || referenceType == "teacher assignment" || referenceType == "draft result" {
				executeSetup(`INSERT INTO class_subject_offerings(class_id,academic_session_id,term_id,subject_id,created_by) VALUES($1,$2,$3,$4,$5)`, classID, sessionID, termID, subjectID, adminID)
			}
			if referenceType == "teacher assignment" {
				executeSetup(`INSERT INTO teacher_subject_assignments(teacher_id,class_id,academic_session_id,term_id,subject_id) VALUES($1,$2,$3,$4,$5)`, fixture.users["teacherA"].ID, classID, sessionID, termID, subjectID)
			}
			if referenceType == "draft result" || referenceType == "empty result batch" || referenceType == "submission history" {
				executeSetup(`INSERT INTO result_batches(class_id,academic_session_id,term_id,subject_id) VALUES($1,$2,$3,$4)`, classID, sessionID, termID, subjectID)
			}
			if referenceType == "draft result" {
				executeSetup(`INSERT INTO results(student_id,subject_id,class_id,academic_session_id,term_id,ca_score,exam_score,total_score,grade,remark) VALUES($1,$2,$3,$4,$5,30,50,80,'A','Excellent')`, fixture.users["studentA"].ID, subjectID, classID, sessionID, termID)
			}
			if referenceType == "submission history" {
				executeSetup(`INSERT INTO result_batch_events(class_id,academic_session_id,term_id,subject_id,action,actor_id) VALUES($1,$2,$3,$4,'submitted',$5)`, classID, sessionID, termID, subjectID, adminID)
			}
			path := subjectManagementPath + "/" + subjectID.String()
			before := academicSessionDeletionSnapshot(t, fixture.db)
			response := fixture.call(t, "admin", http.MethodDelete, path, "")
			expectOwnershipStatus(t, response, http.StatusConflict)
			if !strings.Contains(response.Body.String(), "subject cannot be deleted") {
				t.Fatal("referenced subject did not return a descriptive conflict", response.Body)
			}
			if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
				t.Fatal("rejected deletion changed academic records")
			}

			response = fixture.call(t, "admin", http.MethodPatch, path, `{"name":"Renamed subject"}`)
			expectOwnershipStatus(t, response, http.StatusOK)
			storedSubject, err := repository.NewSubjectRepository(fixture.db).GetByID(t.Context(), subjectID)
			if err != nil || storedSubject.Name != "Renamed subject" {
				t.Fatalf("subject rename was not saved: %v", err)
			}
			// Restore only the renamed subject row to its prior state so a complete
			// snapshot comparison can detect any change to other subjects or history.
			executeSetup(`UPDATE subjects SET name='Referenced subject',updated_at=created_at WHERE id=$1`, subjectID)
			if after := academicSessionDeletionSnapshot(t, fixture.db); after != before {
				t.Fatal("subject rename changed academic records or unrelated subjects")
			}
		})
	}
}
