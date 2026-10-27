
import { useEffect, useRef, useState } from "react";
import {
  ArrowUpRight,
  GraduationCap,
  Plus,
  Search,
  UserRoundPlus,
  UsersRound,
  UserX,
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

import "./StudentEnrollment.css";

const PAGE_SIZE = 5;
const UNASSIGNED = "UNASSIGNED";

type EnrollmentConfiguration = {
  locked: boolean;
  students: { id: string; name: string }[];
};

type StudentPlacement = {
  classID: string;
  className: string;
};


type PromotionChoice = "PROMOTE" | "REPEAT" | "PENDING";

type PromotionDraft = {
  decision: PromotionChoice | "";
  targetClassID: string;
};

type PromotionDecision = {
  id: string;
  student_id: string;
  student_name: string;
  admission_number?: string | null;
  source_class_name: string;
  decision: PromotionChoice;
  target_class_id?: string | null;
  target_class_name?: string | null;
  target_session_start_date: string;
  status: "SAVED" | "FINALIZED";
};

type PromotionListResponse = {
  decisions: PromotionDecision[];
};

type PromotionSaveResponse = {
  saved: number;
  updated: number;
  decisions: PromotionDecision[];
};

type PromotionFinalizationResult = {
  decision_id: string;
  student_id?: string;
  status: "FINALIZED" | "ALREADY_FINALIZED" | "FAILED";
  reason?: string;
};

type PromotionFinalizationResponse = {
  total: number;
  succeeded: number;
  already_finalized: number;
  failed: number;
  results: PromotionFinalizationResult[];
};


export default function StudentEnrollment() {
  const [students, setStudents] = useState<AdminUser[]>([]);
  const [classes, setClasses] = useState<SchoolClass[]>([]);
  const [sessions, setSessions] = useState<AcademicSession[]>([]);
  const [terms, setTerms] = useState<Options["terms"]>([]);

  const [sessionID, setSessionID] = useState("");
  const [termID, setTermID] = useState("");
  const [classID, setClassID] = useState("");
  const [search, setSearch] = useState("");
  const [currentPage, setCurrentPage] = useState(1);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const [activeTermID, setActiveTermID] = useState("");
  const [activeTermLoading, setActiveTermLoading] = useState(false);
  const [activeTermResolved, setActiveTermResolved] = useState(false);

  const [configurations, setConfigurations] = useState<
    Record<string, EnrollmentConfiguration>
  >({});

  const [studentClassMap, setStudentClassMap] = useState<
    Record<string, StudentPlacement>
  >({});

  const [rostersLoading, setRostersLoading] = useState(false);
  const [rostersError, setRostersError] = useState("");
  const [loadedPeriod, setLoadedPeriod] = useState("");

  const [showEnrollForm, setShowEnrollForm] = useState(false);
  const [selectedStudentID, setSelectedStudentID] = useState("");
  const [enrolling, setEnrolling] = useState(false);
  const [enrollmentError, setEnrollmentError] = useState("");
  const [enrollMessage, setEnrollMessage] = useState("");

  const [studentToRemove, setStudentToRemove] =
    useState<AdminUser | null>(null);
  const [removing, setRemoving] = useState(false);

  const enrollmentPending = useRef(false);
  const removalPending = useRef(false);


const [showPromotion, setShowPromotion] = useState(false);
const [promotionDrafts, setPromotionDrafts] = useState<
  Record<string, PromotionDraft>
>({});
const [promotionDecisions, setPromotionDecisions] = useState<
  PromotionDecision[]
>([]);
const [targetSessionStartDate, setTargetSessionStartDate] =
  useState("");
const [promotionLoading, setPromotionLoading] = useState(false);
const [promotionSaving, setPromotionSaving] = useState(false);
const [promotionFinalizing, setPromotionFinalizing] = useState(false);
const [promotionError, setPromotionError] = useState("");
const [promotionMessage, setPromotionMessage] = useState("");
const [selectedDecisionIDs, setSelectedDecisionIDs] = useState<
  string[]
>([]);
const [promotionResults, setPromotionResults] = useState<
  PromotionFinalizationResult[]
>([]);

const [deletingDecisionID, setDeletingDecisionID] = useState("");

const [rosterRefreshKey, setRosterRefreshKey] = useState(0);

const promotionPending = useRef(false);


  const activeSession = sessions.find(
    (session) => session.is_active
  );

  const selectedSessionIsActive =
    !!sessionID && activeSession?.id === sessionID;

  const selectedClass = classes.find(
    (schoolClass) => schoolClass.id === classID
  );

  const selectedClassName = selectedClass?.name ?? "—";
  const periodKey = `${sessionID}:${termID}`;

  const rosterReady =
    !!sessionID &&
    !!termID &&
    !loading &&
    !rostersLoading &&
    !rostersError &&
    loadedPeriod === periodKey;

  const configuration =
    rosterReady && selectedClass
      ? configurations[selectedClass.id] ?? null
      : null;

  const selectedTermIsActive =
    activeTermResolved &&
    !!activeTermID &&
    activeTermID === termID;

  const canManage =
    selectedSessionIsActive &&
    selectedTermIsActive &&
    rosterReady &&
    !activeTermLoading &&
    !enrolling &&
    !removing;

  const canEnroll =
    canManage &&
    !!selectedClass &&
    !!configuration &&
    !configuration.locked;

  // Load students, classes, sessions and terms.
  useEffect(() => {
    let active = true;

    async function load() {
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

        setStudents(
          users.filter((user) => user.role === "STUDENT")
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
              : "Unable to load enrollment information."
          );
        }
      } finally {
        if (active) setLoading(false);
      }
    }

    void load();

    return () => {
      active = false;
    };
  }, []);

  // Automatically select the active term.
  useEffect(() => {
    setActiveTermID("");
    setActiveTermResolved(false);

    if (!activeSession?.id) {
  setActiveTermLoading(false);
  setActiveTermResolved(true);
  return;
}

    let active = true;

    async function loadActiveTerm() {
      setActiveTermLoading(true);

      try {
        const term = await getActiveTerm(activeSession!.id);

        if (active) {
  setActiveTermID(term.id);

  if (sessionID === activeSession?.id) {
    setTermID((previous) => previous || term.id);
  }
}
      } catch {
        if (active) setActiveTermID("");
      } finally {
        if (active) {
          setActiveTermLoading(false);
          setActiveTermResolved(true);
        }
      }
    }

    void loadActiveTerm();

    return () => {
      active = false;
    };
  }, [sessionID, selectedSessionIsActive]);

  // Load class rosters for the selected academic period.
  useEffect(() => {
    if (!sessionID || !termID || loading) {
      setConfigurations({});
      setStudentClassMap({});
      setLoadedPeriod("");
      setRostersError("");
      setRostersLoading(false);
      return;
    }

    let active = true;

    async function loadRosters() {
      setConfigurations({});
      setStudentClassMap({});
      setLoadedPeriod("");
      setRostersError("");
      setRostersLoading(true);

      try {
        const results = await Promise.all(
          classes.map(async (schoolClass) => {
            const query = periodQuery({
              class_id: schoolClass.id,
              academic_session_id: sessionID,
              term_id: termID,
            });

            const configuration =
              await resultRequest<EnrollmentConfiguration>(
                `admin/classes/${schoolClass.id}/term-configuration?${query}`
              );

            return { schoolClass, configuration };
          })
        );

        if (!active) return;

        const nextConfigurations: Record<
          string,
          EnrollmentConfiguration
        > = {};

        const nextPlacements: Record<
          string,
          StudentPlacement
        > = {};

        for (const { schoolClass, configuration } of results) {
          nextConfigurations[schoolClass.id] = configuration;

          for (const student of configuration.students) {
            nextPlacements[student.id] = {
              classID: schoolClass.id,
              className: schoolClass.name,
            };
          }
        }

        setConfigurations(nextConfigurations);
        setStudentClassMap(nextPlacements);
        setLoadedPeriod(`${sessionID}:${termID}`);
      } catch (err) {
        if (active) {
          setRostersError(
            err instanceof Error
              ? err.message
              : "Unable to load student class placements."
          );
        }
      } finally {
        if (active) setRostersLoading(false);
      }
    }

    void loadRosters();

    return () => {
      active = false;
    };
  }, [sessionID, termID, classes, loading, rosterRefreshKey]);

  // Statistics.
  const totalEnrolled = rosterReady
    ? Object.keys(studentClassMap).length
    : 0;

  const enrolledInSelectedClass =
    configuration?.students.length ?? 0;

  const unassignedCount = rosterReady
    ? students.filter(
        (student) => !studentClassMap[student.id]
      ).length
    : 0;

  const availableStudents = rosterReady
    ? students.filter(
        (student) =>
          student.is_active && !studentClassMap[student.id]
      )
    : [];

  // Search and class filter.
  const filteredStudents = students.filter((student) => {
    const term = search.trim().toLowerCase();

    const matchesSearch = [
      student.full_name,
      student.email,
      student.admission_number,
      student.guardian_name,
    ].some((value) =>
      value?.toLowerCase().includes(term)
    );

    const placement = studentClassMap[student.id];

    const matchesClass =
      !classID ||
      (classID === UNASSIGNED
        ? rosterReady && !placement
        : rosterReady && placement?.classID === classID);

    return matchesSearch && matchesClass;
  });

  const totalPages = Math.ceil(
    filteredStudents.length / PAGE_SIZE
  );

  const startIndex = (currentPage - 1) * PAGE_SIZE;

  const visibleStudents = filteredStudents.slice(
    startIndex,
    startIndex + PAGE_SIZE
  );

  const stats = [
    {
      label: "Total Students",
      value: students.length,
      description: "All registered students",
      icon: UsersRound,
      color: "blue",
    },
    {
      label: selectedClass
        ? "Enrolled in Selected Class"
        : "Enrolled This Term",
      value: rosterReady
        ? selectedClass
          ? enrolledInSelectedClass
          : totalEnrolled
        : "—",
      description: selectedClass
        ? "Students in this class and term"
        : "Students assigned to a class this term",
      icon: UserRoundPlus,
      color: "green",
    },
    {
      label: "Unassigned Students",
      value: rosterReady ? unassignedCount : "—",
      description: "No class assignment in the selected term",
      icon: UserX,
      color: "orange",
    },
    {
      label: "Promotion Queue",
      value: "—",
      description: "Promotion workflow pending",
      icon: ArrowUpRight,
      color: "purple",
    },
  ];

  function initials(name: string) {
    return name
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((part) => part[0])
      .join("")
      .toUpperCase();
  }

  function resetEnrollmentForm() {
    setShowEnrollForm(false);
    setSelectedStudentID("");
    setEnrollmentError("");
    setEnrollMessage("");
    setStudentToRemove(null);
  }

  function clearFilters() {
    setSearch("");
    setClassID("");
    setTermID("");
    setCurrentPage(1);
    resetEnrollmentForm();
  }

  // Enroll an unassigned student.
  async function enrollStudent() {
    if (
      !canEnroll ||
      !selectedStudentID ||
      !availableStudents.some(
        (student) => student.id === selectedStudentID
      ) ||
      enrollmentPending.current
    ) {
      return;
    }

    enrollmentPending.current = true;
    setEnrolling(true);
    setEnrollmentError("");
    setEnrollMessage("");

    const targetClassID = classID;
    const targetSessionID = sessionID;
    const targetTermID = termID;
    const targetStudentID = selectedStudentID;

    try {
      const updated = await resultRequest<EnrollmentConfiguration>(
        `admin/classes/${targetClassID}/term-configuration`,
        "PATCH",
        {
          class_id: targetClassID,
          academic_session_id: targetSessionID,
          term_id: targetTermID,
          category: "enrollment",
          operation: "add",
          student_id: targetStudentID,
        }
      );

      const targetClass = classes.find(
        (schoolClass) => schoolClass.id === targetClassID
      );

      setConfigurations((previous) => ({
        ...previous,
        [targetClassID]: updated,
      }));

      if (targetClass) {
        setStudentClassMap((previous) => ({
          ...previous,
          [targetStudentID]: {
            classID: targetClassID,
            className: targetClass.name,
          },
        }));
      }

      setSelectedStudentID("");
      setShowEnrollForm(false);
      setCurrentPage(1);
      setEnrollMessage("Student enrolled successfully.");
    } catch (err) {
      setEnrollmentError(
        err instanceof Error
          ? err.message
          : "Unable to enroll student."
      );
    } finally {
      enrollmentPending.current = false;
      setEnrolling(false);
    }
  }

  // Remove a student's enrollment from the active term.
  async function removeEnrollment() {
    if (
      !studentToRemove ||
      !canManage ||
      removalPending.current
    ) {
      return;
    }

    const placement = studentClassMap[studentToRemove.id];
    if (!placement) return;

    const sourceConfiguration =
      configurations[placement.classID];

    if (!sourceConfiguration || sourceConfiguration.locked) {
      setEnrollmentError(
        "This class-term is locked. Enrollment cannot be removed."
      );
      setStudentToRemove(null);
      return;
    }

    removalPending.current = true;
    setRemoving(true);
    setEnrollmentError("");
    setEnrollMessage("");

    const targetStudentID = studentToRemove.id;
    const targetClassID = placement.classID;

    try {
      const updated = await resultRequest<EnrollmentConfiguration>(
        `admin/classes/${targetClassID}/term-configuration`,
        "PATCH",
        {
          class_id: targetClassID,
          academic_session_id: sessionID,
          term_id: termID,
          category: "enrollment",
          operation: "remove",
          student_id: targetStudentID,
        }
      );

      setConfigurations((previous) => ({
        ...previous,
        [targetClassID]: updated,
      }));

      setStudentClassMap((previous) => {
        const next = { ...previous };
        delete next[targetStudentID];
        return next;
      });

      setStudentToRemove(null);
      setCurrentPage(1);
      setEnrollMessage(
        "Student enrollment removed successfully."
      );
    } catch (err) {
      setEnrollmentError(
        err instanceof Error
          ? err.message
          : "Unable to remove student enrollment."
      );
    } finally {
      removalPending.current = false;
      setRemoving(false);
    }
  }


const sourceStudents = rosterReady && selectedClass
  ? students.filter(
      (student) =>
        studentClassMap[student.id]?.classID === selectedClass.id
    )
  : [];

const activeDestinationSession = sessions.find(
  (session) => session.is_active
);

const eligibleForFinalization = promotionDecisions.filter(
  (decision) =>
    decision.status === "SAVED" &&
    decision.decision !== "PENDING"
);

const promotionBusy =
  promotionLoading ||
  promotionSaving ||
  promotionFinalizing ||
  !!deletingDecisionID;

function updatePromotionDraft(
  studentID: string,
  decision: PromotionChoice
) {
  setPromotionDrafts((previous) => ({
    ...previous,
    [studentID]: {
      decision,
      targetClassID: "",
    },
  }));
}

function updatePromotionTarget(
  studentID: string,
  targetClassID: string
) {
  setPromotionDrafts((previous) => ({
    ...previous,
    [studentID]: {
      decision: "PROMOTE",
      targetClassID,
    },
  }));
}

async function loadPromotionDecisions() {
  if (!selectedClass || !sessionID || !termID) return;

  setPromotionLoading(true);
  setPromotionError("");

  try {
    const query = new URLSearchParams({
      source_class_id: selectedClass.id,
      source_academic_session_id: sessionID,
      source_term_id: termID,
    });

    const response = await resultRequest<PromotionListResponse>(
      `admin/student-promotions/decisions?${query.toString()}`
    );

    setPromotionDecisions(response.decisions);
    setSelectedDecisionIDs([]);

    const drafts: Record<string, PromotionDraft> = {};

    for (const decision of response.decisions) {
      drafts[decision.student_id] = {
        decision: decision.decision,
        targetClassID: decision.target_class_id ?? "",
      };
    }

    setPromotionDrafts(drafts);

    const savedDate = response.decisions[0]?.target_session_start_date;
    if (savedDate) setTargetSessionStartDate(savedDate);
  } catch (err) {
    setPromotionError(
      err instanceof Error
        ? err.message
        : "Unable to load promotion decisions."
    );
  } finally {
    setPromotionLoading(false);
  }
}

async function savePromotionDecisions() {
  if (
    !selectedClass ||
    !sessionID ||
    !termID ||
    !selectedSessionIsActive ||
    !selectedTermIsActive ||
    promotionPending.current
  ) {
    return;
  }

  if (!targetSessionStartDate) {
    setPromotionError("Enter the next session's start date.");
    return;
  }

  const decisions = sourceStudents
    .filter((student) => {
      const existing = promotionDecisions.find(
        (item) => item.student_id === student.id
      );
      return (
        !!promotionDrafts[student.id]?.decision &&
        existing?.status !== "FINALIZED"
      );
    })
    .map((student) => {
      const draft = promotionDrafts[student.id];

      return {
        student_id: student.id,
        decision: draft.decision,
        ...(draft.decision === "PROMOTE"
          ? { target_class_id: draft.targetClassID }
          : {}),
      };
    });

  if (decisions.length === 0) {
    setPromotionError("Choose at least one student decision.");
    return;
  }

  if (
    decisions.some(
      (decision) =>
        decision.decision === "PROMOTE" &&
        (!("target_class_id" in decision) ||
          !decision.target_class_id ||
          decision.target_class_id === selectedClass.id)
    )
  ) {
    setPromotionError(
      "Every promoted student needs a different destination class."
    );
    return;
  }

  promotionPending.current = true;
  setPromotionSaving(true);
  setPromotionError("");
  setPromotionMessage("");

  try {
    const response = await resultRequest<PromotionSaveResponse>(
      "admin/student-promotions/decisions",
      "PUT",
      {
        source_class_id: selectedClass.id,
        source_academic_session_id: sessionID,
        source_term_id: termID,
        target_session_start_date: targetSessionStartDate,
        decisions,
      }
    );

    setPromotionMessage(
      `${response.saved} decision(s) saved, ${response.updated} updated.`
    );

    await loadPromotionDecisions();
  } catch (err) {
    setPromotionError(
      err instanceof Error
        ? err.message
        : "Unable to save promotion decisions."
    );
  } finally {
    promotionPending.current = false;
    setPromotionSaving(false);
  }
}

async function finalizePromotionDecisions() {
  if (
    !activeDestinationSession ||
    !activeTermID ||
    selectedDecisionIDs.length === 0 ||
    promotionPending.current
  ) {
    return;
  }

  const confirmed = window.confirm(
    `Finalize ${selectedDecisionIDs.length} student decision(s)? ` +
    "This will create enrollment records in the active First Term."
  );

  if (!confirmed) return;

  promotionPending.current = true;
  setPromotionFinalizing(true);
  setPromotionError("");
  setPromotionMessage("");
  setPromotionResults([]);

  try {
    const response =
      await resultRequest<PromotionFinalizationResponse>(
        "admin/student-promotions/finalize",
        "POST",
        {
          academic_session_id: activeDestinationSession.id,
          term_id: activeTermID,
          decision_ids: selectedDecisionIDs,
        }
      );

    setPromotionResults(response.results);
    setPromotionMessage(
      `${response.succeeded} finalized, ` +
      `${response.already_finalized} already finalized, ` +
      `${response.failed} failed.`
    );

    setSelectedDecisionIDs([]);
    setRosterRefreshKey((previous) => previous + 1);
    await loadPromotionDecisions();
  } catch (err) {
    setPromotionError(
      err instanceof Error
        ? err.message
        : "Unable to finalize promotions."
    );
  } finally {
    promotionPending.current = false;
    setPromotionFinalizing(false);
  }
}


async function deletePromotionDecision(decision: PromotionDecision) {
  if (
    decision.status !== "SAVED" ||
    promotionPending.current
  ) {
    return;
  }

  const confirmed = window.confirm(
    `Delete the saved ${decision.decision.toLowerCase()} decision for ${decision.student_name}?`
  );

  if (!confirmed) return;

  promotionPending.current = true;
  setDeletingDecisionID(decision.id);
  setPromotionError("");
  setPromotionMessage("");

  try {
    await resultRequest<void>(
      `admin/student-promotions/decisions/${decision.id}`,
      "DELETE"
    );

    setPromotionDecisions((previous) =>
      previous.filter((item) => item.id !== decision.id)
    );

    setPromotionDrafts((previous) => {
      const updated = { ...previous };
      delete updated[decision.student_id];
      return updated;
    });

    setSelectedDecisionIDs((previous) =>
      previous.filter((id) => id !== decision.id)
    );

    setPromotionMessage(
      `Saved promotion decision for ${decision.student_name} deleted successfully.`
    );
  } catch (error) {
    setPromotionError(
      error instanceof Error
        ? error.message
        : "Unable to delete promotion decision."
    );
  } finally {
    promotionPending.current = false;
    setDeletingDecisionID("");
  }
}


  const tableLoading = loading || rostersLoading;

  const tableUnavailable =
    !!sessionID &&
    !!termID &&
    (!rosterReady || !!rostersError);

  return (
    <main className="enrollment-page">
      <header className="enrollment-banner">
        <div>
          <h1>Student Enrollment</h1>
          <p>
            Enroll students, manage class placement, and prepare
            promotion into the next academic period.
          </p>
        </div>

        <GraduationCap
          className="enrollment-banner-icon"
          size={110}
          aria-hidden="true"
        />
      </header>

      <section className="enrollment-stats">
        {stats.map(
          ({ label, value, description, icon: Icon, color }) => (
            <div className="enrollment-stat-card" key={label}>
              <div className={`enrollment-stat-icon ${color}`}>
                <Icon size={27} aria-hidden="true" />
              </div>

              <div>
                <span>{label}</span>
                <strong>
                  {loading || error ? "—" : value}
                </strong>
                <small>{description}</small>
              </div>
            </div>
          )
        )}
      </section>

      <section className="enrollment-panel">
        <div className="enrollment-panel-heading">
          <div>
            <h2>Enrollment Management</h2>
            <p>
              Manage student placements by academic session and term.
            </p>
          </div>
        </div>

        <div className="enrollment-toolbar">
          <label className="enrollment-search">
            <Search size={19} aria-hidden="true" />
            <input
              type="search"
              placeholder="Search by name, admission number or guardian..."
              value={search}
              onChange={(event) => {
                setSearch(event.target.value);
                setCurrentPage(1);
              }}
            />
          </label>

          <label>
            <span>Academic Session</span>
            <select
              value={sessionID}
              disabled={enrolling || removing}
              onChange={(event) => {
                setSessionID(event.target.value);
                setTermID("");
                setClassID("");
                setCurrentPage(1);
                resetEnrollmentForm();
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
            <span>Term</span>
            <select
              value={termID}
              disabled={!sessionID || enrolling || removing}
              onChange={(event) => {
                setTermID(event.target.value);
                setCurrentPage(1);
                resetEnrollmentForm();
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
                    selectedSessionIsActive
                      ? " (Active)"
                      : ""}
                  </option>
                ))}
            </select>
          </label>

          <label>
            <span>Class</span>
            <select
              value={classID}
              disabled={enrolling || removing}
              onChange={(event) => {
                setClassID(event.target.value);
                setCurrentPage(1);
                resetEnrollmentForm();
              }}
            >
              <option value="">All Classes</option>
              <option value={UNASSIGNED}>Unassigned</option>

              {classes.map((schoolClass) => (
                <option key={schoolClass.id} value={schoolClass.id}>
                  {schoolClass.name}
                </option>
              ))}
            </select>
          </label>

          <button
            type="button"
            className="enrollment-clear-button"
            disabled={enrolling || removing}
            onClick={clearFilters}
          >
            Clear Filters
          </button>

          <button
            type="button"
            className="enrollment-primary-button"
            disabled={
              !canEnroll ||
              availableStudents.length === 0
            }
            onClick={() => {
              setShowEnrollForm((previous) => !previous);
              setSelectedStudentID("");
              setEnrollmentError("");
              setEnrollMessage("");
            }}
          >
            <Plus size={18} />
            Enroll Student
          </button>


<button
  type="button"
  className="enrollment-promote-button"
  disabled={
    !rosterReady ||
    !selectedClass ||
    enrolling ||
    removing ||
    promotionBusy
  }
  onClick={() => {
    if (!showPromotion) {
      setShowEnrollForm(false);
      setPromotionError("");
      setPromotionMessage("");
      setPromotionResults([]);
      void loadPromotionDecisions();
    }

    setShowPromotion((previous) => !previous);
  }}
>
  <ArrowUpRight size={18} />
  {showPromotion ? "Close Promotion" : "Promote Students"}
</button>

        </div>

        {showEnrollForm && configuration && (
          <div className="enrollment-form">
            <h3>Enroll Student</h3>
            <p>
              Enrolling into{" "}
              <strong>{selectedClassName}</strong> for the
              selected academic session and term.
            </p>

            <select
              value={selectedStudentID}
              disabled={!canEnroll}
              aria-label="Select student to enroll"
              onChange={(event) =>
                setSelectedStudentID(event.target.value)
              }
            >
              <option value="">Select student</option>

              {availableStudents.map((student) => (
                <option key={student.id} value={student.id}>
                  {student.full_name} —{" "}
                  {student.admission_number ?? "No ID"}
                </option>
              ))}
            </select>

            <button
              type="button"
              className="enrollment-primary-button"
              disabled={!canEnroll || !selectedStudentID}
              onClick={() => void enrollStudent()}
            >
              {enrolling
                ? "Enrolling..."
                : "Confirm Enrollment"}
            </button>
          </div>
        )}


{showPromotion && selectedClass && (
  <div className="promotion-panel">
    <div className="promotion-heading">
      <div>
        <h3>Student Promotion</h3>
        <p>
          Source: <strong>{selectedClass.name}</strong>.
          Save decisions now and finalize when the next
          academic session's First Term is active.
        </p>
      </div>
    </div>

    {promotionError && (
      <p className="enrollment-error" role="alert">
        {promotionError}
      </p>
    )}

    {promotionMessage && (
      <p className="enrollment-success" role="status">
        {promotionMessage}
      </p>
    )}

    {promotionLoading ? (
      <p>Loading promotion decisions...</p>
    ) : (
      <>
        <div className="promotion-settings">
          <label>
            Next Session Start Date
            <input
              type="date"
              value={targetSessionStartDate}
              disabled={
                !selectedSessionIsActive ||
                !selectedTermIsActive ||
                promotionBusy
              }
              onChange={(event) =>
                setTargetSessionStartDate(event.target.value)
              }
            />
          </label>
        </div>

        <div className="enrollment-table-wrapper">
          <table className="enrollment-table">
            <thead>
              <tr>
                <th>Student</th>
                <th>Decision</th>
                <th>Destination Class</th>
                <th>Status</th>
                <th>Finalize</th>
                <th>Delete</th>
              </tr>
            </thead>

            <tbody>
              {sourceStudents.length === 0 ? (
                <tr>
                  <td colSpan={6} className="enrollment-empty">
                    No enrolled students in this class-term.
                  </td>
                </tr>
              ) : (
                sourceStudents.map((student) => {
                  const saved = promotionDecisions.find(
                    (item) => item.student_id === student.id
                  );

                  const draft = promotionDrafts[student.id];
                  const isFinalized =
                    saved?.status === "FINALIZED";

                  const editable =
                    selectedSessionIsActive &&
                    selectedTermIsActive &&
                    !isFinalized &&
                    !promotionBusy;

                  const canSelectForFinalization =
                    saved?.status === "SAVED" &&
                    saved.decision !== "PENDING" &&
                    !promotionBusy;

                  return (
                    <tr key={student.id}>
                      <td>
                        <strong>{student.full_name}</strong>
                        <small>
                          {student.admission_number ?? "—"}
                        </small>
                      </td>

                   <td>
                        <select
                          value={draft?.decision ?? ""}
                          disabled={!editable}
                          onChange={(event) =>
                            updatePromotionDraft(
                              student.id,
                              event.target.value as PromotionChoice
                            )
                          }
                        >
                          <option value="">Choose decision</option>
                          <option value="PROMOTE">Promote</option>
                          <option value="REPEAT">Repeat</option>
                          <option value="PENDING">Pending</option>
                        </select>
                  </td>

                      <td>
                        {draft?.decision === "PROMOTE" ? (
                          <select
                            value={draft.targetClassID}
                            disabled={!editable}
                            onChange={(event) =>
                              updatePromotionTarget(
                                student.id,
                                event.target.value
                              )
                            }
                          >
                            <option value="">Select class</option>
                            {classes
                              .filter(
                                (schoolClass) =>
                                  schoolClass.id !== selectedClass.id
                              )
                              .map((schoolClass) => (
                                <option
                                  key={schoolClass.id}
                                  value={schoolClass.id}
                                >
                                  {schoolClass.name}
                                </option>
                              ))}
                          </select>
                        ) : draft?.decision === "REPEAT" ? (
                          selectedClass.name
                        ) : (
                          "—"
                        )}
                      </td>



                      <td>{saved?.status ?? "Not saved"}</td>

                      <td>
                        <input
                          type="checkbox"
                          aria-label={`Select ${student.full_name} for finalization`}
                          checked={selectedDecisionIDs.includes(
                            saved?.id ?? ""
                          )}
                          disabled={!canSelectForFinalization}
                          onChange={(event) => {
                            if (!saved) return;

                            setSelectedDecisionIDs((previous) =>
                              event.target.checked
                                ? [...previous, saved.id]
                                : previous.filter(
                                    (id) => id !== saved.id
                                  )
                            );
                          }}
                        />
                      </td>

                          <td>
  {saved?.status === "SAVED" && (
    <button
      type="button"
      className="promotion-delete-button"
      disabled={
        promotionBusy ||
        !!deletingDecisionID ||
        enrolling ||
        removing
      }
      onClick={() => void deletePromotionDecision(saved)}
    >
      {deletingDecisionID === saved.id
        ? "Deleting..."
        : "Delete"}
    </button>
  )}
</td>



                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>

        <div className="promotion-actions">
          <button
            type="button"
            className="enrollment-primary-button"
            disabled={
              !selectedSessionIsActive ||
              !selectedTermIsActive ||
              promotionBusy ||
              sourceStudents.length === 0
            }
            onClick={() => void savePromotionDecisions()}
          >
            {promotionSaving ? "Saving..." : "Save Decisions"}
          </button>

          <button
            type="button"
            className="enrollment-promote-button"
            disabled={
              !activeDestinationSession ||
              !activeTermID ||
              selectedDecisionIDs.length === 0 ||
              promotionBusy
            }
            onClick={() => void finalizePromotionDecisions()}
          >
            {promotionFinalizing
              ? "Finalizing..."
              : `Finalize Selected (${selectedDecisionIDs.length})`}
          </button>
        </div>

        {promotionResults.length > 0 && (
          <div className="promotion-results">
            <h4>Finalization Results</h4>

            {promotionResults.map((result) => {
              const decision = promotionDecisions.find(
                (item) => item.id === result.decision_id
              );

              return (
                <p key={result.decision_id}>
                  <strong>
                    {decision?.student_name ??
                      result.student_id ??
                      result.decision_id}
                  </strong>
                  {" — "}
                  {result.status}
                  {result.reason ? `: ${result.reason}` : ""}
                </p>
              );
            })}
          </div>
        )}
      </>
    )}
  </div>
)}


        {error && (
          <p className="enrollment-error" role="alert">
            {error}
          </p>
        )}

        {rostersLoading && (
          <p className="enrollment-loading" role="status">
            Loading student class placements...
          </p>
        )}

        {rostersError && (
          <p className="enrollment-error" role="alert">
            {rostersError}
          </p>
        )}

        {enrollmentError && (
          <p className="enrollment-error" role="alert">
            {enrollmentError}
          </p>
        )}

        {enrollMessage && (
          <p className="enrollment-success" role="status">
            {enrollMessage}
          </p>
        )}

        {configuration?.locked && (
          <p className="enrollment-loading" role="status">
            This class-term is locked after result submission.
            Enrollment changes are unavailable.
          </p>
        )}

        {sessionID &&
          termID &&
          rosterReady &&
          (!selectedSessionIsActive ||
            !selectedTermIsActive) && (
            <p className="enrollment-loading" role="status">
              This is not the active academic period.
              Enrollment records are available for viewing only.
            </p>
          )}

        <div className="enrollment-table-wrapper">
          <table className="enrollment-table">
            <thead>
              <tr>
                <th>Student</th>
                <th>Admission No</th>
                <th>Class</th>
                <th>Status</th>
                <th>Guardian</th>
                <th>Date Enrolled</th>
                <th>Actions</th>
              </tr>
            </thead>

            <tbody>
              {tableLoading ||
              tableUnavailable ||
              !sessionID ||
              !termID ||
              filteredStudents.length === 0 ? (
                <tr>
                  <td colSpan={7} className="enrollment-empty">
                    {tableLoading
                      ? "Loading students..."
                      : !sessionID || !termID
                        ? "Select an academic session and term."
                        : tableUnavailable
                          ? "Unable to display class placements."
                          : "No students found."}
                  </td>
                </tr>
              ) : (
                visibleStudents.map((student) => {
                  const placement =
                    studentClassMap[student.id];

                  const canRemove =
                    !!placement &&
                    canManage &&
                    !configurations[placement.classID]?.locked;

                  return (
                    <tr key={student.id}>
                      <td>
                        <div className="enrollment-student">
                          <span className="enrollment-avatar">
                            {initials(student.full_name)}
                          </span>

                          <div>
                            <strong>
                              {student.full_name}
                            </strong>
                            <small>{student.email}</small>
                          </div>
                        </div>
                      </td>

                      <td>
                        {student.admission_number ?? "—"}
                      </td>

                      <td>
                        {placement?.className ?? "—"}
                      </td>

                      <td>
                        {placement ? (
                          <span className="enrollment-status-enrolled">
                            Enrolled
                          </span>
                        ) : (
                          <span className="enrollment-status-pending">
                            Unassigned
                          </span>
                        )}
                      </td>

                      <td>
                        <strong>
                          {student.guardian_name || "—"}
                        </strong>
                        <small>
                          {student.guardian_phone || ""}
                        </small>
                      </td>

                      <td>—</td>

                      <td>
                        <button
                          type="button"
                          className="enrollment-remove-button"
                          disabled={!canRemove}
                          onClick={() =>
                            setStudentToRemove(student)
                          }
                        >
                          Remove
                        </button>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>

        {!tableLoading &&
          !tableUnavailable &&
          !error &&
          !!sessionID &&
          !!termID &&
          filteredStudents.length > 0 && (
            <div className="enrollment-pagination">
              <p>
                Showing {startIndex + 1}–
                {Math.min(
                  startIndex + PAGE_SIZE,
                  filteredStudents.length
                )}{" "}
                of {filteredStudents.length} students
              </p>

              <div>
                <button
                  type="button"
                  disabled={currentPage === 1}
                  onClick={() =>
                    setCurrentPage((page) => page - 1)
                  }
                >
                  Previous
                </button>

                <span>
                  {currentPage} / {totalPages}
                </span>

                <button
                  type="button"
                  disabled={currentPage >= totalPages}
                  onClick={() =>
                    setCurrentPage((page) => page + 1)
                  }
                >
                  Next
                </button>
              </div>
            </div>
          )}

        {/* Remove Enrollment Confirmation */}
        {studentToRemove && (
          <div className="enrollment-confirm-overlay">
            <div
              className="enrollment-confirm-dialog"
              role="alertdialog"
              aria-modal="true"
              aria-labelledby="remove-enrollment-title"
              aria-describedby="remove-enrollment-description"
            >
              <h3 id="remove-enrollment-title">
                Remove Enrollment?
              </h3>

              <p id="remove-enrollment-description">
                Remove{" "}
                <strong>
                  {studentToRemove.full_name}
                </strong>{" "}
                from{" "}
                <strong>
                  {studentClassMap[studentToRemove.id]?.className}
                </strong>{" "}
                for the selected academic term?
              </p>

              <p>
                The student's account and admission number will not
                be deleted. Removal is blocked if academic records exist.
              </p>

              <div className="enrollment-confirm-actions">
                <button
                  type="button"
                  disabled={removing}
                  onClick={() =>
                    setStudentToRemove(null)
                  }
                >
                  Cancel
                </button>

                <button
                  type="button"
                  className="enrollment-confirm-remove"
                  disabled={removing}
                  onClick={() =>
                    void removeEnrollment()
                  }
                >
                  {removing
                    ? "Removing..."
                    : "Confirm Removal"}
                </button>
              </div>
            </div>
          </div>
        )}
      </section>
    </main>
  );
}
