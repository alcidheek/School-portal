
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  CalendarCheck,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  RefreshCw,
  Search,
  UsersRound,
  XCircle,
  ChartNoAxesColumn,
} from "lucide-react";

import {
  getUsers,
  getClasses,
  getAcademicSessions,
  type AdminUser,
  type SchoolClass,
  type AcademicSession,
  type AcademicTerm,
} from "../../services/adminService";

import { getAcademicTerms } from "../../services/academicService";

import {
  getAttendance,
  type AttendanceRecord,
  type AttendanceFilters,
} from "../../services/attendanceService";

import "./Attendance.css";

const PAGE_SIZE = 8;

function getErrorMessage(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "An unexpected error occurred.";
}

function formatDate(dateValue: string): string {
  const date = new Date(`${dateValue.slice(0, 10)}T12:00:00`);

  if (Number.isNaN(date.getTime())) return "—";

  return date.toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    timeZone: "UTC",
  });
}

function formatTermName(name: string): string {
  return name
    .toLowerCase()
    .split("_")
    .map((word) => word[0].toUpperCase() + word.slice(1))
    .join(" ");
}

function getInitials(name: string): string {
  return name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase();
}

function Attendance() {
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [classes, setClasses] = useState<SchoolClass[]>([]);
  const [sessions, setSessions] = useState<AcademicSession[]>([]);
  const [terms, setTerms] = useState<AcademicTerm[]>([]);
  const [attendance, setAttendance] = useState<AttendanceRecord[]>([]);

  const [selectedSessionID, setSelectedSessionID] = useState("");
  const [selectedTermID, setSelectedTermID] = useState("");
  const [selectedClassID, setSelectedClassID] = useState("");
  const [selectedDate, setSelectedDate] = useState("");

  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);

  const [loading, setLoading] = useState(true);
  const [loadingTerms, setLoadingTerms] = useState(false);
  const [loadingAttendance, setLoadingAttendance] = useState(false);
  const [refreshVersion, setRefreshVersion] = useState(0);
  const [error, setError] = useState("");

  const usersByID = useMemo(
    () => new Map(users.map((user) => [user.id, user])),
    [users]
  );

  const classesByID = useMemo(
    () =>
      new Map(classes.map((schoolClass) => [
        schoolClass.id,
        schoolClass,
      ])),
    [classes]
  );

  const sessionsByID = useMemo(
    () => new Map(sessions.map((session) => [
      session.id,
      session,
    ])),
    [sessions]
  );

  const termsByID = useMemo(
    () => new Map(terms.map((term) => [term.id, term])),
    [terms]
  );

  // Load users, classes and academic sessions.
  useEffect(() => {
    let cancelled = false;

    async function loadInitialData() {
      setLoading(true);
      setError("");

      try {
        const [userRecords, classRecords, sessionRecords] =
          await Promise.all([
            getUsers(),
            getClasses(),
            getAcademicSessions(),
          ]);

        if (cancelled) return;

        setUsers(userRecords);
        setClasses(classRecords);
        setSessions(sessionRecords);

        const activeSession = sessionRecords.find(
          (session) => session.is_active
        );

        setSelectedSessionID(activeSession?.id ?? "");
      } catch (requestError) {
        if (!cancelled) {
          setError(getErrorMessage(requestError));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void loadInitialData();

    return () => {
      cancelled = true;
    };
  }, []);

  // Load terms for the selected academic session.
  useEffect(() => {
    let cancelled = false;

    setTerms([]);
    setSelectedTermID("");

    if (!selectedSessionID) {
      setLoadingTerms(false);
      return;
    }

    async function loadTerms() {
      setLoadingTerms(true);
      setError("");

      try {
        const termRecords = await getAcademicTerms(
          selectedSessionID
        );

        if (cancelled) return;

        setTerms(termRecords);

        const activeTerm = termRecords.find(
          (term) => term.is_active
        );

        setSelectedTermID(activeTerm?.id ?? "");
      } catch (requestError) {
        if (!cancelled) {
          setError(getErrorMessage(requestError));
        }
      } finally {
        if (!cancelled) setLoadingTerms(false);
      }
    }

    void loadTerms();

    return () => {
      cancelled = true;
    };
  }, [selectedSessionID]);

  const loadAttendance = useCallback(
    async (cancelled: () => boolean) => {
      const filters: AttendanceFilters = {};

      if (selectedSessionID) {
        filters.academic_session_id = selectedSessionID;
      }

      if (selectedTermID) {
        filters.term_id = selectedTermID;
      }

      if (selectedClassID) {
        filters.class_id = selectedClassID;
      }

      if (selectedDate) {
        filters.date = selectedDate;
      }

      setLoadingAttendance(true);
      setError("");

      try {
        const records = await getAttendance(filters);

        if (!cancelled()) {
          setAttendance(records);
        }
      } catch (requestError) {
        if (!cancelled()) {
          setAttendance([]);
          setError(getErrorMessage(requestError));
        }
      } finally {
        if (!cancelled()) {
          setLoadingAttendance(false);
        }
      }
    },
    [
      selectedSessionID,
      selectedTermID,
      selectedClassID,
      selectedDate,
    ]
  );

  useEffect(() => {
    let cancelled = false;

    // Wait for the term dropdown when a session is selected.
    if (loading || loadingTerms) return;

    void loadAttendance(() => cancelled);

    return () => {
      cancelled = true;
    };
  }, [
    loading,
    loadingTerms,
    loadAttendance,
    refreshVersion,
  ]);

  // Reset pagination when filters change.
  useEffect(() => {
    setPage(1);
  }, [
    selectedSessionID,
    selectedTermID,
    selectedClassID,
    selectedDate,
    search,
  ]);

  const filteredRecords = useMemo(() => {
    const searchValue = search.trim().toLowerCase();

    if (!searchValue) return attendance;

    return attendance.filter((record) => {
      const student = usersByID.get(record.student_id);

      return (
        (student?.full_name ?? "")
          .toLowerCase()
          .includes(searchValue) ||
        (student?.admission_number ?? "")
          .toLowerCase()
          .includes(searchValue)
      );
    });
  }, [attendance, search, usersByID]);

  const totalRecords = filteredRecords.length;

  const presentCount = filteredRecords.filter(
    (record) => record.status === "PRESENT"
  ).length;

  const absentCount = filteredRecords.filter(
    (record) => record.status === "ABSENT"
  ).length;

  const attendanceRate =
    totalRecords > 0
      ? Math.round((presentCount / totalRecords) * 100)
      : 0;

  const totalPages = Math.max(
    1,
    Math.ceil(filteredRecords.length / PAGE_SIZE)
  );

  const currentPage = Math.min(page, totalPages);

  const visibleRecords = filteredRecords.slice(
    (currentPage - 1) * PAGE_SIZE,
    currentPage * PAGE_SIZE
  );

  function clearFilters() {
    setSelectedSessionID("");
    setSelectedTermID("");
    setSelectedClassID("");
    setSelectedDate("");
    setSearch("");
  }

  return (
    <main className="attendance-page">
      {/* Banner */}
      <section className="attendance-banner">
        <div>
          <h1>Attendance Management</h1>
          <p>
            View attendance records submitted by teachers
            across classes and academic periods.
          </p>
        </div>

        <CalendarCheck
          className="attendance-banner-icon"
          size={86}
          aria-hidden="true"
        />
      </section>

      {/* Statistics */}
      <section className="attendance-stats">
        <div className="attendance-stat-card">
          <div className="attendance-stat-icon blue">
            <UsersRound size={28} />
          </div>
          <div>
            <span>Total Records</span>
            <strong>
              {loading || loadingAttendance
                ? "—"
                : totalRecords}
            </strong>
            <small>Attendance entries</small>
          </div>
        </div>

        <div className="attendance-stat-card">
          <div className="attendance-stat-icon green">
            <CheckCircle2 size={28} />
          </div>
          <div>
            <span>Present</span>
            <strong>
              {loading || loadingAttendance
                ? "—"
                : presentCount}
            </strong>
            <small>Present entries</small>
          </div>
        </div>

        <div className="attendance-stat-card">
          <div className="attendance-stat-icon red">
            <XCircle size={28} />
          </div>
          <div>
            <span>Absent</span>
            <strong>
              {loading || loadingAttendance
                ? "—"
                : absentCount}
            </strong>
            <small>Absent entries</small>
          </div>
        </div>

        <div className="attendance-stat-card">
          <div className="attendance-stat-icon purple">
            <ChartNoAxesColumn size={28} />
          </div>
          <div>
            <span>Attendance Rate</span>
            <strong>
              {loading || loadingAttendance
                ? "—"
                : `${attendanceRate}%`}
            </strong>
            <small>Present out of total</small>
          </div>
        </div>
      </section>

      {/* Attendance overview */}
      <section className="attendance-panel">
        <div className="attendance-panel-heading">
          <div>
            <h2>Attendance Overview</h2>
            <p>
              View teacher-submitted attendance records.
              Filter by academic period, class or date.
            </p>
          </div>

          <button
            type="button"
            className="attendance-refresh-button"
            onClick={() =>
              setRefreshVersion((version) => version + 1)
            }
            disabled={loading || loadingAttendance}
          >
            <RefreshCw size={17} />
            Refresh
          </button>
        </div>

        <div className="attendance-filters">
          <label className="attendance-filter">
            Academic Session
            <select
              value={selectedSessionID}
              onChange={(event) =>
                setSelectedSessionID(event.target.value)
              }
            >
              <option value="">All Sessions</option>
              {sessions.map((session) => (
                <option key={session.id} value={session.id}>
                  {session.name}
                  {session.is_active ? " (Active)" : ""}
                </option>
              ))}
            </select>
          </label>

          <label className="attendance-filter">
            Academic Term
            <select
              value={selectedTermID}
              disabled={!selectedSessionID || loadingTerms}
              onChange={(event) =>
                setSelectedTermID(event.target.value)
              }
            >
              <option value="">All Terms</option>
              {terms.map((term) => (
                <option key={term.id} value={term.id}>
                  {formatTermName(term.name)}
                  {term.is_active ? " (Active)" : ""}
                </option>
              ))}
            </select>
          </label>

          <label className="attendance-filter">
            Class
            <select
              value={selectedClassID}
              onChange={(event) =>
                setSelectedClassID(event.target.value)
              }
            >
              <option value="">All Classes</option>
              {classes.map((schoolClass) => (
                <option
                  key={schoolClass.id}
                  value={schoolClass.id}
                >
                  {schoolClass.name}
                </option>
              ))}
            </select>
          </label>

          <label className="attendance-filter">
            Date
            <input
              type="date"
              value={selectedDate}
              onChange={(event) =>
                setSelectedDate(event.target.value)
              }
            />
          </label>

          <button
            type="button"
            className="attendance-clear-button"
            onClick={clearFilters}
          >
            Clear Filters
          </button>
        </div>

        <div className="attendance-toolbar">
          <div className="attendance-search">
            <Search size={18} />
            <input
              type="search"
              placeholder="Search by student name or admission number..."
              aria-label="Search students"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>

          <span className="attendance-readonly">
            Read-only attendance records
          </span>
        </div>

        {error && (
          <p className="attendance-error" role="alert">
            {error}
          </p>
        )}

        <div className="attendance-table-wrapper">
          <table className="attendance-table">
            <thead>
              <tr>
                <th>#</th>
                <th>Student</th>
                <th>Class</th>
                <th>Date</th>
                <th>Status</th>
                <th>Recorded By</th>
                <th>Session</th>
                <th>Term</th>
              </tr>
            </thead>

            <tbody>
              {loading || loadingAttendance || loadingTerms ? (
                <tr>
                  <td colSpan={8} className="attendance-empty">
                    Loading attendance records...
                  </td>
                </tr>
              ) : visibleRecords.length === 0 ? (
                <tr>
                  <td colSpan={8} className="attendance-empty">
                    No attendance records match the selected filters.
                  </td>
                </tr>
              ) : (
                visibleRecords.map((record, index) => {
                  const student = usersByID.get(
                    record.student_id
                  );

                  const teacher = usersByID.get(
                    record.recorded_by
                  );

                  const schoolClass = classesByID.get(
                    record.class_id
                  );

                  const session = sessionsByID.get(
                    record.academic_session_id
                  );

                  const term = termsByID.get(record.term_id);

                  const studentName =
                    student?.full_name ?? "Unknown Student";

                  return (
                    <tr key={record.id}>
                      <td>
                        {(currentPage - 1) * PAGE_SIZE +
                          index +
                          1}
                      </td>

                      <td>
                        <div className="attendance-student">
                          <span className="attendance-avatar">
                            {getInitials(studentName)}
                          </span>

                          <div>
                            <strong>{studentName}</strong>
                            <small>
                              {student?.admission_number ??
                                "No admission number"}
                            </small>
                          </div>
                        </div>
                      </td>

                      <td>
                        {schoolClass?.name ?? "Unknown Class"}
                      </td>

                      <td>{formatDate(record.date)}</td>

                      <td>
                        <span
                          className={`attendance-status ${
                            record.status === "PRESENT"
                              ? "present"
                              : "absent"
                          }`}
                        >
                          {record.status}
                        </span>
                      </td>

                      <td>
                        {teacher?.full_name ??
                          "Unknown Teacher"}
                      </td>

                      <td>
                        {session?.name ?? "Unknown Session"}
                      </td>

                      <td>
                        {term
                          ? formatTermName(term.name)
                          : "—"}
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>

        <div className="attendance-pagination">
          <span>
            Showing{" "}
            {totalRecords === 0
              ? 0
              : (currentPage - 1) * PAGE_SIZE + 1}
            {" – "}
            {Math.min(currentPage * PAGE_SIZE, totalRecords)}
            {" of "}
            {totalRecords} records
          </span>

          <div>
            <button
              type="button"
              disabled={currentPage === 1}
              onClick={() => setPage(currentPage - 1)}
              aria-label="Previous page"
            >
              <ChevronLeft size={17} />
            </button>

            {Array.from(
              { length: totalPages },
              (_, index) => index + 1
            ).map((pageNumber) => (
              <button
                key={pageNumber}
                type="button"
                className={
                  currentPage === pageNumber ? "active" : ""
                }
                onClick={() => setPage(pageNumber)}
              >
                {pageNumber}
              </button>
            ))}

            <button
              type="button"
              disabled={currentPage === totalPages}
              onClick={() => setPage(currentPage + 1)}
              aria-label="Next page"
            >
              <ChevronRight size={17} />
            </button>
          </div>
        </div>
      </section>
    </main>
  );
}

export default Attendance;
