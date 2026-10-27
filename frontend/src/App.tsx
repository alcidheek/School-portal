
import { Routes, Route, Navigate } from "react-router-dom";

import SessionMonitor from "./components/SessionMonitor";
import RoleGuard from "./components/RoleGuard";

import ForgotPasswordPage from "./pages/ForgotPasswordPage";
import ResetPasswordPage from "./pages/ResetPasswordPage";
import WelcomeSplash from "./components/WelcomeSplash";

// Admin layout and pages
import AdminLayout from "./components/admin/AdminLayout";
import AdminDashboard from "./pages/admin/AdminDashboard";
import UsersPage from "./pages/admin/UsersPage";
import AddUserPage from "./pages/admin/AddUserPage";
import StudentEnrollment from "./pages/admin/StudentEnrollment";
import TeacherAssignment from "./pages/admin/TeacherAssignment";
import Classes from "./pages/admin/Classes";
import Subjects from "./pages/admin/Subjects";
import Attendance from "./pages/admin/Attendance";
import AcademicSessions from "./pages/admin/AcademicSessions";
import Settings from "./pages/admin/Settings";

// Results
import ResultWorkflow from "./pages/results/ResultWorkflow";
import PublishedResults from "./pages/results/PublishedResults";

// Teacher and Student dashboards
import TeacherDashboard from "./pages/teacher/TeacherDashboard";
import StudentDashboard from "./pages/student/StudentDashboard";

function App() {
  return (
    <>
      <SessionMonitor />

      <Routes>
        {/* PUBLIC ROUTES */}

       <Route path="/" element={<WelcomeSplash />} />

        <Route
          path="/forgot-password"
          element={<ForgotPasswordPage />}
        />

        <Route
          path="/reset-password"
          element={<ResetPasswordPage />}
        />

        {/* ADMIN ROUTES */}

        <Route element={<RoleGuard role="ADMIN" />}>
          <Route path="/admin" element={<AdminLayout />}>
            <Route
              path="dashboard"
              element={<AdminDashboard />}
            />

            <Route
              path="users"
              element={<UsersPage />}
            />

            <Route
              path="users/new"
              element={<AddUserPage />}
            />

            <Route
              path="student-enrollments"
              element={<StudentEnrollment />}
            />

            <Route
              path="teacher-assignments"
              element={<TeacherAssignment />}
            />

            <Route
              path="classes"
              element={<Classes />}
            />

            <Route
              path="subjects"
              element={<Subjects />}
            />

            <Route
              path="academic-sessions"
              element={<AcademicSessions />}
            />

            <Route
              path="attendance"
              element={<Attendance />}
            />

            <Route
              path="results"
              element={<ResultWorkflow />}
            />

            {/* SETTINGS */}

            <Route
              path="settings"
              element={<Settings />}
            />

            {/* Preserve existing Account Security URL */}

            <Route
              path="settings/account-security"
              element={
                <Navigate
                  to="/admin/settings?tab=account-security"
                  replace
                />
              }
            />
          </Route>
        </Route>

        {/* TEACHER ROUTES */}

        <Route element={<RoleGuard role="TEACHER" />}>
          <Route
            path="/teacher/dashboard"
            element={<TeacherDashboard />}
          />

          <Route
            path="/teacher/results"
            element={<ResultWorkflow teacher />}
          />
        </Route>

        {/* STUDENT ROUTES */}

        <Route element={<RoleGuard role="STUDENT" />}>
          <Route
            path="/student/dashboard"
            element={<StudentDashboard />}
          />

          <Route
            path="/student/results"
            element={<PublishedResults />}
          />
        </Route>
      </Routes>
    </>
  );
}

export default App;
