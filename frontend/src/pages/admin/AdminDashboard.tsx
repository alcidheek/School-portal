import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useScrollReveal } from "../../hooks/useScrollReveal";
import {
  ArrowRight,
  BellRing,
  CalendarCheck2,
  GraduationCap,
  Megaphone,
  School,
  UserPlus,
  Users,
} from "lucide-react";

import "./AdminDashboard.css";
import StatCard from "../../components/admin/StatCard";
import bannerImage from "../../assets/blueprint-school-banner.png";

import {
  getAcademicSessions,
  getActiveTerm,
  getClasses,
  getSubjects,
  getUsers,
  type AcademicSession,
  type AcademicTerm,
  type AdminUser,
  type SchoolClass,
  type Subject,
} from "../../services/adminService";

function formatTermName(name?: string) {
  if (!name) return "No Active Term";
  return name
    .toLowerCase()
    .replaceAll("_", " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function AdminDashboard() {
  const navigate = useNavigate();

  // Animate dashboard sections when scrolling into view.
  useScrollReveal();

  const [users, setUsers] = useState<AdminUser[]>([]);
  const [classes, setClasses] = useState<SchoolClass[]>([]);
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [academicSessions, setAcademicSessions] = useState<AcademicSession[]>(
    []
  );
  const [activeTerm, setActiveTerm] = useState<AcademicTerm | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;

    async function loadDashboardData() {
      try {
        const [usersData, classesData, subjectsData, sessionsData] =
          await Promise.all([
            getUsers(),
            getClasses(),
            getSubjects(),
            getAcademicSessions(),
          ]);

        if (!active) return;

        setUsers(usersData);
        setClasses(classesData);
        setSubjects(subjectsData);
        setAcademicSessions(sessionsData);

        const currentSession = sessionsData.find((session) => session.is_active);

        if (currentSession) {
          try {
            const term = await getActiveTerm(currentSession.id);
            if (active) setActiveTerm(term);
          } catch {
            if (active) setActiveTerm(null);
          }
        }
      } catch (error) {
        console.error("Failed to load dashboard data:", error);
      } finally {
        if (active) setLoading(false);
      }
    }

    void loadDashboardData();

    return () => {
      active = false;
    };
  }, []);

  const activeSession = academicSessions.find((session) => session.is_active);

  const studentCount = useMemo(
    () => users.filter((user) => user.role === "STUDENT").length,
    [users]
  );

  const teacherCount = useMemo(
    () => users.filter((user) => user.role === "TEACHER").length,
    [users]
  );

  const attendanceSummary = {
    present: 0,
    absent: 0,

  };

const totalAttendance =
  attendanceSummary.present + attendanceSummary.absent;


  const attendancePercent =
    totalAttendance > 0
      ? Math.round((attendanceSummary.present / totalAttendance) * 100)
      : 0;


const quickActions = [
  {
    title: "Add User",
    description: "Create a new staff or student account",
    icon: UserPlus,
    className: "action-blue",
    onClick: () => navigate("/admin/users/new"),
  },
  {
    title: "Enroll Student",
    description: "Manage student enrollment for a term",
    icon: GraduationCap,
    className: "action-green",
    onClick: () => navigate("/admin/student-enrollments"),
  },
  {
    title: "Assign Teacher",
    description: "Assign teachers to classes",
    icon: Users,
    className: "action-purple",
    onClick: () => navigate("/admin/teacher-assignments"),
  },
  {
    title: "View Attendance",
    description: "View student attendance records",
    icon: CalendarCheck2,
    className: "action-orange",
    onClick: () => navigate("/admin/attendance"),
  },
];


  return (
    <main className="admin-dashboard-page">
      <section className="dashboard-banner">
        <div className="dashboard-banner__content">
          <h1>Welcome back, Administrator</h1>
          <p>
            Your school is running smoothly. Here&apos;s an overview of
            today&apos;s activities.
          </p>
        </div>

        <div className="dashboard-banner__image-wrap">
          <img
            src={bannerImage}
            alt="BluePrint school building"
            className="dashboard-banner__image"
          />
          <div className="dashboard-banner__quote">
            <p>
              “Empowering learners today
              <br />
              for a brighter tomorrow.”
            </p>
            <span />
          </div>
        </div>
      </section>

      <section className="dashboard-stats-grid">
        <StatCard
          title="Total Students"
          value={studentCount}
          description="Student accounts"
          icon={Users}
          iconClassName="stat-icon-blue"
          loading={loading}
        />
        <StatCard
          title="Total Teachers"
          value={teacherCount}
          description="Teaching staff"
          icon={GraduationCap}
          iconClassName="stat-icon-green"
          loading={loading}
        />
        <StatCard
          title="Active Classes"
          value={classes.length}
          description="Available classes"
          icon={School}
          iconClassName="stat-icon-purple"
          loading={loading}
        />
        <StatCard
          title="Attendance Today"
          value={`${attendancePercent}%`}
          description={
            totalAttendance > 0
              ? `${attendanceSummary.present} / ${totalAttendance} present`
              : "No attendance recorded yet"
          }
          icon={CalendarCheck2}
          iconClassName="stat-icon-teal"
          loading={false}
        />
      </section>

      <section className="dashboard-main-grid scroll-reveal">
        <section className="dashboard-card attendance-card">
          <div className="dashboard-card__header">
            <div className="dashboard-card__title-group">
              <h2>Attendance Overview</h2>
            </div>

            <button type="button" className="ghost-select">
              Today
            </button>
          </div>

          <div className="attendance-overview">
            <div
              className="attendance-ring"
              style={{
                background: `conic-gradient(
                  #19d3a7 0% ${attendanceSummary.present ? (attendanceSummary.present / totalAttendance) * 100 : 0}%,
                  #f4b740 ${attendanceSummary.present ? (attendanceSummary.present / totalAttendance) * 100 : 0}% ${totalAttendance ? ((attendanceSummary.present + attendanceSummary.absent) / totalAttendance) * 100 : 0}%,
                  #4aa4ff ${totalAttendance ? ((attendanceSummary.present + attendanceSummary.absent) / totalAttendance) * 100 : 0}% 100%
                )`,
              }}
            >
              <div className="attendance-ring__inner">
                <strong>{attendancePercent}%</strong>
                <span>Present Today</span>
              </div>
            </div>

            <div className="attendance-legend">
              <div className="attendance-legend__item">
                <span className="legend-dot legend-green" />
                <span className="legend-label">Present</span>
                <strong>{attendanceSummary.present}</strong>
                <span className="legend-percent">
                  {totalAttendance
                    ? `${Math.round(
                        (attendanceSummary.present / totalAttendance) * 100
                      )}%`
                    : "0%"}
                </span>
              </div>

              <div className="attendance-legend__item">
                <span className="legend-dot legend-yellow" />
                <span className="legend-label">Absent</span>
                <strong>{attendanceSummary.absent}</strong>
                <span className="legend-percent">
                  {totalAttendance
                    ? `${Math.round(
                        (attendanceSummary.absent / totalAttendance) * 100
                      )}%`
                    : "0%"}
                </span>
              </div>


            </div>
          </div>
        </section>

        <section className="dashboard-card quick-actions-card">
          <div className="dashboard-card__header">
            <div className="dashboard-card__title-group">
              <h2>Quick Actions</h2>
            </div>
          </div>

          <div className="quick-actions-grid">
            {quickActions.map((action) => {
              const Icon = action.icon;

              return (
                <button
                  key={action.title}
                  type="button"
                  className={`quick-action-tile ${action.className}`}
                  onClick={action.onClick}
                >
                  <div className="quick-action-tile__icon">
                    <Icon size={22} aria-hidden="true" />
                  </div>

                  <div className="quick-action-tile__body">
                    <strong>{action.title}</strong>
                    <span>{action.description}</span>
                  </div>

                  <ArrowRight size={18} className="quick-action-tile__arrow" />
                </button>
              );
            })}
          </div>
        </section>
      </section>

      <section className="dashboard-bottom-grid scroll-reveal">
        <section className="dashboard-card info-card">
          <div className="dashboard-card__header">
            <div className="dashboard-card__title-group">
              <h2>Announcements</h2>
            </div>
            <button type="button" className="card-link-button">
              View All
            </button>
          </div>

          <div className="dashboard-empty-state">
            <Megaphone size={20} aria-hidden="true" />
            <p>No announcements available yet.</p>
          </div>
        </section>

        <section className="dashboard-card info-card">
          <div className="dashboard-card__header">
            <div className="dashboard-card__title-group">
              <h2>Recent Activity</h2>
            </div>
            <button type="button" className="card-link-button">
              View All
            </button>
          </div>

          <div className="dashboard-activity-placeholder">
            <div className="activity-item">
              <span className="activity-icon">
                <BellRing size={16} aria-hidden="true" />
              </span>
              <div>
                <strong>Current Session</strong>
                <p>{activeSession?.name ?? "No active academic session"}</p>
              </div>
            </div>

            <div className="activity-item">
              <span className="activity-icon">
                <CalendarCheck2 size={16} aria-hidden="true" />
              </span>
              <div>
                <strong>Current Term</strong>
                <p>{formatTermName(activeTerm?.name)}</p>
              </div>
            </div>
          </div>
        </section>
      </section>
    </main>
  );
}

export default AdminDashboard;
