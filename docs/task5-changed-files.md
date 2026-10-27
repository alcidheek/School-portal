# Task 5 changed files

Source changes were compared against hashes recorded before Task 5. Existing
migrations and frontend styling remain unchanged; no source files were deleted.

Modified (29):

```text
backend/config/config.go
backend/internal/auth/jwt.go
backend/internal/handler/active_account.go
backend/internal/handler/api_response_test.go
backend/internal/handler/password_reset_handler.go
backend/internal/middleware/auth.go
backend/internal/middleware/auth_test.go
backend/internal/model/user.go
backend/internal/repository/password_reset_repository.go
backend/internal/repository/password_reset_repository_test.go
backend/internal/service/academic_authorization.go
backend/internal/service/account_security_service.go
backend/internal/service/account_security_service_test.go
backend/internal/service/password_reset_service.go
backend/internal/service/password_reset_service_test.go
backend/internal/service/user_service.go
backend/routes/authorization_test.go
backend/routes/methods_test.go
backend/routes/routes.go
docs/account-security.md
docs/authorization.md
frontend/src/App.tsx
frontend/src/components/admin/AdminSidebar.tsx
frontend/src/pages/LoginPage.tsx
frontend/src/pages/student/StudentDashboard.tsx
frontend/src/pages/teacher/TeacherDashboard.tsx
frontend/src/services/accountSecurityService.ts
frontend/src/services/adminService.ts
frontend/src/services/authService.ts
```

Created (17):

```text
backend/config/config_test.go
backend/internal/auth/jwt_test.go
backend/internal/handler/logout_handler.go
backend/internal/middleware/token_security_test.go
backend/internal/repository/token_repository.go
backend/internal/service/authentication_security_test.go
backend/internal/service/token_service.go
backend/migrations/014_authentication_security.sql
backend/routes/authentication_security_test.go
docs/authentication-security.md
docs/task5-changed-files.md
frontend/src/components/LogoutButton.tsx
frontend/src/components/SessionMonitor.tsx
frontend/src/pages/ForgotPasswordPage.tsx
frontend/src/pages/ResetPasswordPage.tsx
frontend/src/services/authSession.ts
frontend/tests/passwordReset.test.mjs
```

Temporary editing scripts and the isolated PostgreSQL cluster/logs are removed
after verification and excluded. No changes to backend/.env, migrations 001-013,
Go/npm dependencies, or unrelated academic/frontend modules.
