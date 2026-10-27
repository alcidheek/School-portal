
import { useEffect, useState } from "react";
import {
  BookOpen,
  GraduationCap,
  Search,
  UserRoundCheck,
  UsersRound,
} from "lucide-react";

import {
  getUsers,
  getClasses,
  getAcademicSessions,
  getActiveTerm,
  type AdminUser,
  type SchoolClass,
  type AcademicSession,
} from "../../services/adminService";

import {
  resultRequest,
  periodQuery,
  type Options,
} from "../../services/resultService";

import "./TeacherAssignment.css";

type Reference = {
  id: string;
  name: string;
};

type SubjectTeacher = {
  subject_id: string;
  teacher: Reference;
};

type TermConfiguration = {
  locked: boolean;
  subjects: Reference[];
  class_teacher: Reference | null;
  subject_teachers: SubjectTeacher[];
};

type AssignmentRow = {
  id: string;
  teacherName: string;
  role: string;
  subject: string;
  className: string;
};

export default function TeacherAssignment() {
  const [teachers, setTeachers] = useState<AdminUser[]>([]);
  const [classes, setClasses] = useState<SchoolClass[]>([]);
  const [sessions, setSessions] = useState<AcademicSession[]>([]);
  const [terms, setTerms] = useState<Options["terms"]>([]);

  const [sessionID, setSessionID] = useState("");
  const [termID, setTermID] = useState("");
  const [classID, setClassID] = useState("");
  const [activeTermID, setActiveTermID] = useState("");

  const [configuration, setConfiguration] =
    useState<TermConfiguration | null>(null);

  const [loadedPeriod, setLoadedPeriod] = useState("");
  const [loading, setLoading] = useState(true);
  const [configurationLoading, setConfigurationLoading] =
    useState(false);
  const [saving, setSaving] = useState(false);

  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const [search, setSearch] = useState("");
  const [showForm, setShowForm] = useState(false);

  const [assignmentType, setAssignmentType] = useState<
    "CLASS" | "SUBJECT"
  >("CLASS");

  const [teacherID, setTeacherID] = useState("");
  const [subjectID, setSubjectID] = useState("");

  const activeSession = sessions.find(
    (session) => session.is_active
  );

  const selectedClass = classes.find(
    (schoolClass) => schoolClass.id === classID
  );

  const periodKey = `${sessionID}:${termID}:${classID}`;

  const configurationReady =
    !!configuration &&
    loadedPeriod === periodKey &&
    !configurationLoading;

  const isActivePeriod =
    !!sessionID &&
    sessionID === activeSession?.id &&
    !!termID &&
    termID === activeTermID;

  const canAssign =
    configurationReady &&
    isActivePeriod &&
    !configuration.locked &&
    !saving;

  // Load page options.
  useEffect(() => {
    let active = true;

    async function loadOptions() {
      try {
        const [
          users,
          schoolClasses,
          academicSessions,
          resultOptions,
        ] = await Promise.all([
          getUsers(),
          getClasses(),
          getAcademicSessions(),
          resultRequest<Options>("admin/result-options"),
        ]);

        if (!active) return;

        setTeachers(
          users.filter((user) => user.role === "TEACHER")
        );
        setClasses(schoolClasses);
        setSessions(academicSessions);
        setTerms(resultOptions.terms);

        const current = academicSessions.find(
          (session) => session.is_active
        );

        setSessionID(current?.id ?? "");
      } catch (err) {
        if (active) {
          setError(
            err instanceof Error
              ? err.message
              : "Unable to load teacher assignment options."
          );
        }
      } finally {
        if (active) setLoading(false);
      }
    }

    void loadOptions();

    return () => {
      active = false;
    };
  }, []);

  // Select the active term by default.
  useEffect(() => {
    let active = true;
    setActiveTermID("");

    if (!sessionID || sessionID !== activeSession?.id) {
      return;
    }

    async function loadActiveTerm() {
      try {
        const term = await getActiveTerm(sessionID);

        if (active) {
          setActiveTermID(term.id);
          setTermID((previous) => previous || term.id);
        }
      } catch {
        if (active) setActiveTermID("");
      }
    }

    void loadActiveTerm();

    return () => {
      active = false;
    };
  }, [sessionID, activeSession?.id]);

  // Load term-specific assignments.
  useEffect(() => {
    let active = true;

    setConfiguration(null);
    setLoadedPeriod("");
    setError("");
    setMessage("");

    if (!sessionID || !termID || !classID) {
      setConfigurationLoading(false);
      return;
    }

    async function loadConfiguration() {
      setConfigurationLoading(true);

      try {
        const query = periodQuery({
          class_id: classID,
          academic_session_id: sessionID,
          term_id: termID,
        });

        const response =
          await resultRequest<TermConfiguration>(
            `admin/classes/${classID}/term-configuration?${query}`
          );

        if (!active) return;

        setConfiguration(response);
        setLoadedPeriod(`${sessionID}:${termID}:${classID}`);
      } catch (err) {
        if (active) {
          setError(
            err instanceof Error
              ? err.message
              : "Unable to load teacher assignments."
          );
        }
      } finally {
        if (active) setConfigurationLoading(false);
      }
    }

    void loadConfiguration();

    return () => {
      active = false;
    };
  }, [sessionID, termID, classID]);

  function resetForm() {
    setShowForm(false);
    setAssignmentType("CLASS");
    setTeacherID("");
    setSubjectID("");
  }

  function resetFeedback() {
    setError("");
    setMessage("");
  }

  async function saveAssignment() {
    if (!canAssign || !teacherID || !classID) return;

    if (assignmentType === "SUBJECT" && !subjectID) {
      setError("Please select a subject.");
      return;
    }

    const existingTeacher =
      assignmentType === "CLASS"
        ? configuration?.class_teacher
        : configuration?.subject_teachers.find(
            (assignment) => assignment.subject_id === subjectID
          )?.teacher;

    if (existingTeacher && existingTeacher.id !== teacherID) {
      const confirmed = window.confirm(
        `Replace ${existingTeacher.name} with the selected teacher?`
      );

      if (!confirmed) return;
    }

    setSaving(true);
    resetFeedback();

    try {
      const endpoint =
        assignmentType === "CLASS"
          ? "admin/class-teacher-assignments"
          : "admin/teacher-subject-assignments";

      const body = {
        teacher_id: teacherID,
        class_id: classID,
        academic_session_id: sessionID,
        term_id: termID,
        ...(assignmentType === "SUBJECT"
          ? { subject_id: subjectID }
          : {}),
      };

      await resultRequest(endpoint, "POST", body);

      const query = periodQuery({
        class_id: classID,
        academic_session_id: sessionID,
        term_id: termID,
      });

      const updated = await resultRequest<TermConfiguration>(
        `admin/classes/${classID}/term-configuration?${query}`
      );

      setConfiguration(updated);
      setMessage("Teacher assignment saved successfully.");
      resetForm();
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Unable to save teacher assignment."
      );
    } finally {
      setSaving(false);
    }
  }

  const assignments: AssignmentRow[] = [];

  if (configurationReady && selectedClass) {
    if (configuration.class_teacher) {
      assignments.push({
        id: `class-${configuration.class_teacher.id}`,
        teacherName: configuration.class_teacher.name,
        role: "Class Teacher",
        subject: "—",
        className: selectedClass.name,
      });
    }

    for (const assignment of configuration.subject_teachers) {
      const subject = configuration.subjects.find(
        (item) => item.id === assignment.subject_id
      );

      assignments.push({
        id: `subject-${assignment.subject_id}`,
        teacherName: assignment.teacher.name,
        role: "Subject Teacher",
        subject: subject?.name ?? "Unknown Subject",
        className: selectedClass.name,
      });
    }
  }

  const filteredAssignments = assignments.filter((item) =>
    [
      item.teacherName,
      item.role,
      item.subject,
      item.className,
    ].some((value) =>
      value.toLowerCase().includes(search.toLowerCase().trim())
    )
  );

  const subjectAssignmentCount =
    configurationReady
      ? configuration.subject_teachers.length
      : "—";

  const stats = [
    {
      title: "Total Teachers",
      value: loading ? "—" : teachers.length,
      icon: UsersRound,
      color: "blue",
    },
    {
      title: "Class Teacher",
      value: configurationReady
        ? configuration.class_teacher
          ? "1"
          : "0"
        : "—",
      icon: UserRoundCheck,
      color: "green",
    },
    {
      title: "Subject Assignments",
      value: subjectAssignmentCount,
      icon: BookOpen,
      color: "orange",
    },
    {
      title: "Assigned This Class",
      value: configurationReady
        ? new Set(
            assignments.map((item) => item.teacherName)
          ).size
        : "—",
      icon: GraduationCap,
      color: "purple",
    },
  ];

  return (
    <main className="teacher-assignment-page">
      <header className="teacher-assignment-banner">
        <div>
          <h1>Teacher Assignment</h1>
          <p>
            Manage class teachers and subject teachers
            for each academic term.
          </p>
        </div>

        <GraduationCap
          size={100}
          className="teacher-assignment-banner-icon"
          aria-hidden="true"
        />
      </header>

      <section className="teacher-assignment-stats">
        {stats.map(({ title, value, icon: Icon, color }) => (
          <div className="teacher-assignment-stat" key={title}>
            <div className={`teacher-assignment-stat-icon ${color}`}>
              <Icon size={24} />
            </div>
            <div>
              <span>{title}</span>
              <strong>{value}</strong>
            </div>
          </div>
        ))}
      </section>

      <section className="teacher-assignment-panel">
        <div className="teacher-assignment-heading">
          <div>
            <h2>Assignment Management</h2>
            <p>
              Select an academic period and class to manage
              its assigned teachers.
            </p>
          </div>
        </div>

        <div className="teacher-assignment-filters">
          <label>
            Academic Session
            <select
              value={sessionID}
              disabled={saving}
              onChange={(event) => {
                setSessionID(event.target.value);
                setTermID("");
                setClassID("");
                resetForm();
              }}
            >
              <option value="">Select Session</option>
              {sessions.map((session) => (
                <option key={session.id} value={session.id}>
                  {session.name}
                  {session.is_active ? " (Active)" : ""}
                </option>
              ))}
            </select>
          </label>

          <label>
            Term
            <select
              value={termID}
              disabled={!sessionID || saving}
              onChange={(event) => {
                setTermID(event.target.value);
                resetForm();
              }}
            >
              <option value="">Select Term</option>
              {terms
                .filter(
                  (term) =>
                    term.academic_session_id === sessionID
                )
                .map((term) => (
                  <option key={term.id} value={term.id}>
                    {term.name.replaceAll("_", " ")}
                    {term.id === activeTermID &&
                    sessionID === activeSession?.id
                      ? " (Active)"
                      : ""}
                  </option>
                ))}
            </select>
          </label>

          <label>
            Class
            <select
              value={classID}
              disabled={!termID || saving}
              onChange={(event) => {
                setClassID(event.target.value);
                resetForm();
              }}
            >
              <option value="">Select Class</option>
              {classes.map((schoolClass) => (
                <option key={schoolClass.id} value={schoolClass.id}>
                  {schoolClass.name}
                </option>
              ))}
            </select>
          </label>
        </div>

        <div className="teacher-assignment-actions">
          <div className="teacher-assignment-search">
            <Search size={18} />
            <input
              type="search"
              placeholder="Search assigned teachers or subjects..."
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>

          <button
            type="button"
            className="teacher-assignment-primary"
            disabled={!canAssign || teachers.length === 0}
            onClick={() => {
              setShowForm((previous) => !previous);
              resetFeedback();
            }}
          >
            {showForm ? "Close Form" : "+ Assign Teacher"}
          </button>
        </div>

        {configurationReady && !isActivePeriod && (
          <p className="teacher-assignment-notice">
            This is a historical academic period.
            Assignments are available for viewing only.
          </p>
        )}

        {configurationReady && configuration.locked && (
          <p className="teacher-assignment-notice">
            This class-term configuration is locked.
            Teacher assignments cannot be modified.
          </p>
        )}

        {error && (
          <p className="teacher-assignment-error" role="alert">
            {error}
          </p>
        )}

        {message && (
          <p className="teacher-assignment-success" role="status">
            {message}
          </p>
        )}

        {showForm && canAssign && (
          <div className="teacher-assignment-form">
            <h3>Assign Teacher</h3>
            <p>
              Assigning to <strong>{selectedClass?.name}</strong>
            </p>

            <div className="teacher-assignment-form-grid">
              <label>
                Assignment Type
                <select
                  value={assignmentType}
                  onChange={(event) => {
                    setAssignmentType(
                      event.target.value as "CLASS" | "SUBJECT"
                    );
                    setSubjectID("");
                  }}
                >
                  <option value="CLASS">Class Teacher</option>
                  <option value="SUBJECT">Subject Teacher</option>
                </select>
              </label>

              <label>
                Teacher
                <select
                  value={teacherID}
                  onChange={(event) =>
                    setTeacherID(event.target.value)
                  }
                >
                  <option value="">Select Teacher</option>
                  {teachers
                    .filter((teacher) => teacher.is_active)
                    .map((teacher) => (
                      <option key={teacher.id} value={teacher.id}>
                        {teacher.full_name}
                      </option>
                    ))}
                </select>
              </label>

              {assignmentType === "SUBJECT" && (
                <label>
                  Subject
                  <select
                    value={subjectID}
                    onChange={(event) =>
                      setSubjectID(event.target.value)
                    }
                  >
                    <option value="">Select Subject</option>
                    {configuration?.subjects.map((subject) => (
                      <option key={subject.id} value={subject.id}>
                        {subject.name}
                      </option>
                    ))}
                  </select>
                </label>
              )}
            </div>

            <button
              type="button"
              className="teacher-assignment-primary"
              disabled={
                saving ||
                !teacherID ||
                (assignmentType === "SUBJECT" && !subjectID)
              }
              onClick={() => void saveAssignment()}
            >
              {saving ? "Saving..." : "Confirm Assignment"}
            </button>
          </div>
        )}

        <div className="teacher-assignment-table-wrapper">
          <table className="teacher-assignment-table">
            <thead>
              <tr>
                <th>Teacher</th>
                <th>Role</th>
                <th>Subject</th>
                <th>Class</th>
                <th>Status</th>
              </tr>
            </thead>

            <tbody>
              {!configurationReady ||
              filteredAssignments.length === 0 ? (
                <tr>
                  <td
                    colSpan={5}
                    className="teacher-assignment-empty"
                  >
                    {loading || configurationLoading
                      ? "Loading teacher assignments..."
                      : !classID || !termID
                        ? "Select a term and class to view assignments."
                        : !configurationReady
                          ? "Unable to display assignments."
                          : "No teacher assignments found."}
                  </td>
                </tr>
              ) : (
                filteredAssignments.map((assignment) => (
                  <tr key={assignment.id}>
                    <td>
                      <strong>{assignment.teacherName}</strong>
                    </td>

                    <td>
                      <span className="teacher-assignment-role">
                        {assignment.role}
                      </span>
                    </td>

                    <td>{assignment.subject}</td>
                    <td>{assignment.className}</td>

                    <td>
                      <span className="teacher-assignment-status">
                        Assigned
                      </span>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>
    </main>
  );
}
