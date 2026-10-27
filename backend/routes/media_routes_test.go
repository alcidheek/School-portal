package routes

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alcidheek/school-portal/internal/auth"
	"github.com/alcidheek/school-portal/internal/handler"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

// Exercise real JWT, active-account and revocation middleware without a server.
type mediaAuthDB struct {
	user    model.User
	revoked bool
}

func (d *mediaAuthDB) Connect(context.Context) (driver.Conn, error) { return d, nil }
func (d *mediaAuthDB) Driver() driver.Driver                        { return d }
func (d *mediaAuthDB) Open(string) (driver.Conn, error)             { return d, nil }
func (d *mediaAuthDB) Close() error                                 { return nil }
func (d *mediaAuthDB) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (d *mediaAuthDB) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (d *mediaAuthDB) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(q, "SELECT NOT EXISTS") {
		return &mediaAuthRows{values: []driver.Value{d.revoked}}, nil
	}
	if strings.Contains(q, "FROM users") && len(args) == 1 && args[0].Value == d.user.ID.String() {
		return &mediaAuthRows{values: []driver.Value{d.user.ID.String(), "Test user", "user@example.com", "1234567", "unused", string(d.user.Role), nil, nil, nil, nil, d.user.IsActive, time.Now(), time.Now()}}, nil
	}
	if strings.Contains(q, "FROM school_profile") || strings.Contains(q, "INSERT INTO school_profile") {
		return &mediaAuthRows{values: []driver.Value{int64(1), "School", "Motto", "contact@example.com", "1234567", "Address", "", time.Now(), time.Now()}}, nil
	}
	return nil, errors.New("unexpected query")
}

type mediaAuthRows struct {
	values []driver.Value
	done   bool
}

func (r *mediaAuthRows) Columns() []string { return make([]string, len(r.values)) }
func (r *mediaAuthRows) Close() error      { return nil }
func (r *mediaAuthRows) Next(v []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(v, r.values)
	return nil
}

type routeMediaFake struct {
	id    uuid.UUID
	calls int
	image []byte
}

func (m *routeMediaFake) UploadSchoolLogo(ctx context.Context, input io.Reader) error {
	m.calls++
	_, err := io.ReadAll(input)
	return err
}
func (m *routeMediaFake) DownloadSchoolLogo(context.Context) (io.ReadCloser, error) {
	m.calls++
	return io.NopCloser(bytes.NewReader(m.image)), nil
}
func (m *routeMediaFake) DeleteSchoolLogo(context.Context) error { m.calls++; return nil }
func (m *routeMediaFake) UploadProfilePicture(ctx context.Context, id uuid.UUID, input io.Reader) error {
	m.id = id
	return m.UploadSchoolLogo(ctx, input)
}
func (m *routeMediaFake) DownloadProfilePicture(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	m.id = id
	return m.DownloadSchoolLogo(ctx)
}
func (m *routeMediaFake) DeleteProfilePicture(ctx context.Context, id uuid.UUID) error {
	m.id = id
	return m.DeleteSchoolLogo(ctx)
}

func TestMediaRoutesSecurityAndCompatibility(t *testing.T) {
	const secret = "media-route-test-secret"
	d := &mediaAuthDB{user: model.User{ID: uuid.New(), Role: model.RoleAdmin, IsActive: true}}
	db := sql.OpenDB(d)
	defer db.Close()
	users := handler.NewUserHandler(service.NewUserService(repository.NewUserRepository(db), secret))
	school := handler.NewSchoolProfileHandler(service.NewSchoolProfileService(repository.NewSchoolProfileRepository(db)))
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	m := &routeMediaFake{image: imageData.Bytes()}
	router := SetupRoutesWithMedia(users, nil, nil, nil, nil, nil, nil, nil, school, handler.NewMediaHandler(m), secret)
	endpoints := []struct {
		method, path string
		status       int
		admin        bool
	}{
		{"PUT", "/api/v1/admin/school-profile/logo", 200, true},
		{"GET", "/api/v1/school-profile/logo", 200, false},
		{"DELETE", "/api/v1/admin/school-profile/logo", 204, true},
		{"PUT", "/api/v1/users/me/profile-picture", 200, false},
		{"GET", "/api/v1/users/me/profile-picture", 200, false},
		{"DELETE", "/api/v1/users/me/profile-picture", 204, false},
	}
	call := func(method, path string, authenticated bool) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		contentType := ""
		if method == "PUT" && strings.Contains(path, "picture") || method == "PUT" && strings.HasSuffix(path, "/logo") {
			mw := multipart.NewWriter(&body)
			part, err := mw.CreateFormFile("file", "image.png")
			if err != nil {
				t.Fatal(err)
			}
			part.Write(m.image)
			mw.Close()
			contentType = mw.FormDataContentType()
		} else if method == "PUT" {
			body.WriteString(`{"name":"School","motto":"Motto","email":"contact@example.com","phone":"1234567","address":"Address","website":""}`)
		}
		r := httptest.NewRequest(method, path, &body)
		r.Header.Set("Content-Type", contentType)
		if authenticated {
			token, err := auth.GenerateToken(&d.user, secret)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, ep := range endpoints {
		before := m.calls
		w := call(ep.method, ep.path, false)
		if w.Code != 401 || m.calls != before {
			t.Fatalf("missing auth %s: %d %s", ep.path, w.Code, w.Body)
		}
	}
	for _, role := range []model.Role{model.RoleAdmin, model.RoleTeacher, model.RoleStudent} {
		d.user.Role = role
		for _, ep := range endpoints {
			want := ep.status
			if ep.admin && role != model.RoleAdmin {
				want = 403
			}
			path := ep.path
			if strings.Contains(path, "/users/me/") {
				path += "?user_id=" + uuid.NewString()
			}
			before := m.calls
			w := call(ep.method, path, true)
			if w.Code != want {
				t.Fatalf("%s %s %s: %d %s", role, ep.method, path, w.Code, w.Body)
			}
			if want == 403 && m.calls != before {
				t.Fatal("forbidden service call")
			}
			if strings.Contains(path, "/users/me/") && m.id != d.user.ID {
				t.Fatal("profile ownership bypass")
			}
		}
	}
	d.user.Role = model.RoleAdmin
	for _, ep := range endpoints {
		d.user.IsActive = false
		w := call(ep.method, ep.path, true)
		if w.Code != 403 {
			t.Fatalf("inactive: %d", w.Code)
		}
		d.user.IsActive = true
		d.revoked = true
		w = call(ep.method, ep.path, true)
		if w.Code != 401 {
			t.Fatalf("revoked: %d", w.Code)
		}
		d.revoked = false
	}
	for _, ep := range []struct{ method, path string }{{"GET", "/api/v1/admin/test"}, {"GET", "/api/v1/me"}, {"GET", "/api/v1/admin/school-profile"}, {"PUT", "/api/v1/admin/school-profile"}} {
		w := call(ep.method, ep.path, true)
		if w.Code != 200 {
			t.Fatalf("existing %s %s: %d %s", ep.method, ep.path, w.Code, w.Body)
		}
	}
}
