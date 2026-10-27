
import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";

import {
  BookOpen,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  ClipboardList,
  Clock3,
  Eye,
  FileCheck2,
  GraduationCap,
  Layers,
  Search,
  Send,
  XCircle,
} from "lucide-react";

import { getClasses } from "../../services/adminService.ts";

import {
  editableBatch,
  periodQuery,
  resultRequest,
  type Options,
  type Period,
  type Reference,
  type Review,
  type Score,
} from "../../services/resultService.ts";

import "./Results.css";

const PAGE_SIZE = 6;

type BatchStatus =
  | "missing"
  | "draft"
  | "submitted"
  | "approved"
  | "rejected";

function formatDate(value?: string | null): string {
  if (!value) return "—";

  const date = new Date(value);

  if (Number.isNaN(date.getTime())) return "—";

  return date.toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

function formatDateTime(value?: string | null): string {
  if (!value) return "—";

  const date = new Date(value);

  if (Number.isNaN(date.getTime())) return "—";

  return date.toLocaleString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatStatus(status: string): string {
  return status.charAt(0).toUpperCase() + status.slice(1);
}

function getErrorMessage(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "Unable to process results.";
}

/* =========================================
   EXISTING TEACHER SCORE FORM
========================================= */

function ScoreForm({
  student,
  score,
  disabled,
  save,
}: {
  student: Reference;
  score?: Score;
  disabled: boolean;
  save: (ca: number, exam: number) => Promise<void>;
}) {
  const [ca, setCA] = useState(
    score ? String(score.ca_score) : ""
  );

  const [exam, setExam] = useState(
    score ? String(score.exam_score) : ""
  );

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        void save(Number(ca), Number(exam));
      }}
    >
      <fieldset
        disabled={disabled}
        className="result-filters"
      >
        <span>{student.name}</span>

        <label>
          CA (0–40)
          <input
            aria-label={`CA for ${student.name}`}
            type="number"
            min="0"
            max="40"
            step="0.01"
            required
            value={ca}
            onChange={(event) =>
              setCA(event.target.value)
            }
          />
        </label>

        <label>
          Exam (0–60)
          <input
            aria-label={`Exam for ${student.name}`}
            type="number"
            min="0"
            max="60"
            step="0.01"
            required
            value={exam}
            onChange={(event) =>
              setExam(event.target.value)
            }
          />
        </label>

        <span>
          {score
            ? `${score.total_score} · ${score.grade} · ${score.remark}`
            : "No draft saved"}
        </span>

        <button type="submit">
          {score ? "Save correction" : "Save draft"}
        </button>
      </fieldset>
    </form>
  );
}

/* =========================================
   RESULTS WORKFLOW
========================================= */

function ResultWorkflow({
  teacher = false,
}: {
  teacher?: boolean;
}) {
  const role = teacher ? "teacher" : "admin";

  const [options, setOptions] = useState<Options>({
    sessions: [],
    terms: [],
    assignments: [],
  });

  const [classes, setClasses] = useState<Reference[]>([]);

  const [sessionID, setSessionID] = useState("");
  const [classID, setClassID] = useState("");
  const [subjectID, setSubjectID] = useState("");
  const [termID, setTermID] = useState("");

  const [review, setReview] = useState<Review | null>(null);

  const [reason, setReason] =
    useState<Record<string, string>>({});

  const [busy, setBusy] = useState(false);
  const [loadingOptions, setLoadingOptions] = useState(true);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  // Admin-only interface state.
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [subjectFilter, setSubjectFilter] = useState("all");

  const [selectedBatchSubjectID, setSelectedBatchSubjectID] =
    useState("");

  const [currentPage, setCurrentPage] = useState(1);

  const pending = useRef(false);

  /* =========================================
     LOAD RESULT OPTIONS
  ========================================= */

  useEffect(() => {
    let active = true;

    async function loadOptions() {
      setLoadingOptions(true);

      try {
        const [resultOptions, classRecords] =
          await Promise.all([
            resultRequest<Options>(
              `${role}/result-options`
            ),
            teacher
              ? Promise.resolve([])
              : getClasses(),
          ]);

        if (!active) return;

        setOptions(resultOptions);
        setClasses(classRecords);
      } catch (requestError) {
        if (active) {
          setError(getErrorMessage(requestError));
        }
      } finally {
        if (active) {
          setLoadingOptions(false);
        }
      }
    }

    void loadOptions();

    return () => {
      active = false;
    };
  }, [role, teacher]);

  /* =========================================
     SELECTED ACADEMIC PERIOD
  ========================================= */

  const period: Period = {
    class_id: classID,
    academic_session_id: sessionID,
    term_id: termID,
    ...(teacher ? { subject_id: subjectID } : {}),
  };

  const assignments = options.assignments.filter(
    (assignment) =>
      assignment.academic_session_id === sessionID &&
      assignment.term_id === termID
  );

  const availableTerms = options.terms.filter(
    (term) =>
      term.academic_session_id === sessionID
  );

  const selectedSession = options.sessions.find(
    (session) => session.id === sessionID
  );

  const selectedClass = classes.find(
    (schoolClass) => schoolClass.id === classID
  );

  const selectedTerm = options.terms.find(
    (term) => term.id === termID
  );

  const editable = editableBatch(review);

  /* =========================================
     RESET AND RELOAD
  ========================================= */

  function reset() {
    setReview(null);
    setError("");
    setMessage("");
    setReason({});
    setSearch("");
    setStatusFilter("all");
    setSubjectFilter("all");
    setSelectedBatchSubjectID("");
    setCurrentPage(1);
  }

  async function load() {
    const response = await resultRequest<Review>(
      `${role}/result-batches?${periodQuery(period)}`
    );

    setReview(response);

    if (!teacher) {
      setSelectedBatchSubjectID((previousSubjectID) => {
        const subjectStillExists = response.batches.some(
          (batch) =>
            batch.subject_id === previousSubjectID
        );

        return subjectStillExists
          ? previousSubjectID
          : response.batches[0]?.subject_id ?? "";
      });
    }
  }

  async function run(
    operation: () => Promise<void>,
    success = ""
  ) {
    if (pending.current) return;

    pending.current = true;
    setBusy(true);
    setError("");
    setMessage("");

    try {
      await operation();
      setMessage(success);
    } catch (requestError) {
      setError(getErrorMessage(requestError));
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  /* =========================================
     TEACHER SCORE OPERATIONS
  ========================================= */

  async function save(
    student: Reference,
    score: Score | undefined,
    ca: number,
    exam: number
  ) {
    await run(async () => {
      await resultRequest(
        `teacher/results${score ? `/${score.id}` : ""}`,
        score ? "PATCH" : "POST",
        {
          ...period,
          student_id: student.id,
          ca_score: ca,
          exam_score: exam,
        }
      );

      await load();
    }, "Draft saved.");
  }

  /* =========================================
     SUBJECT BATCH TRANSITIONS
  ========================================= */

  async function transition(
    action: "submit" | "approve" | "reject",
    subject = subjectID
  ) {
    if (
      action === "reject" &&
      !(reason[subject] ?? "").trim()
    ) {
      setError("A rejection reason is required.");
      return;
    }

    await run(async () => {
      await resultRequest(
        `${
          action === "submit" ? "teacher" : "admin"
        }/result-batches/${action}`,
        "POST",
        {
          ...period,
          subject_id: subject,
          ...(action === "reject"
            ? { reason: reason[subject].trim() }
            : {}),
        }
      );

      await load();
    },
    action === "submit"
      ? "Full batch submitted for review."
      : action === "approve"
        ? "Subject batch approved successfully."
        : "Subject batch rejected successfully."
    );
  }

  async function publishResults() {
    await run(async () => {
      await resultRequest(
        "admin/result-publications",
        "POST",
        period
      );

      await load();
    }, "Entire class-term published successfully.");
  }

  /* =========================================
     ADMIN SUMMARY STATISTICS
  ========================================= */

  const allBatches = review?.batches ?? [];

  const totalBatches = allBatches.length;

  const submittedCount = allBatches.filter(
    (batch) => batch.status === "submitted"
  ).length;

  const approvedCount = allBatches.filter(
    (batch) => batch.status === "approved"
  ).length;

  const rejectedCount = allBatches.filter(
    (batch) => batch.status === "rejected"
  ).length;

  const canPublish =
    !!review &&
    !review.publication &&
    allBatches.length > 0 &&
    allBatches.every(
      (batch) => batch.status === "approved"
    );

  /* =========================================
     ADMIN SEARCH AND FILTERS
  ========================================= */

  const filteredBatches = useMemo(() => {
    const searchValue = search.trim().toLowerCase();

    return (review?.batches ?? []).filter((batch) => {
      const matchesSearch =
        batch.subject_name
          .toLowerCase()
          .includes(searchValue);

      const matchesStatus =
        statusFilter === "all" ||
        batch.status === statusFilter;

      const matchesSubject =
        subjectFilter === "all" ||
        batch.subject_id === subjectFilter;

      return (
        matchesSearch &&
        matchesStatus &&
        matchesSubject
      );
    });
  }, [
    review,
    search,
    statusFilter,
    subjectFilter,
  ]);

  const totalPages = Math.max(
    1,
    Math.ceil(filteredBatches.length / PAGE_SIZE)
  );

  const safePage = Math.min(currentPage, totalPages);

  const visibleBatches = filteredBatches.slice(
    (safePage - 1) * PAGE_SIZE,
    safePage * PAGE_SIZE
  );

  const selectedBatch = allBatches.find(
    (batch) =>
      batch.subject_id === selectedBatchSubjectID
  );

  const selectedBatchScores = review?.results.filter(
    (score) =>
      score.subject.id === selectedBatchSubjectID
  ) ?? [];

  const selectedBatchHistory = review?.events.filter(
    (event) =>
      event.subject_id === selectedBatchSubjectID
  ) ?? [];

  const studentsWithScores = selectedBatch
    ? review?.students.filter((student) =>
        selectedBatchScores.some(
          (score) => score.student.id === student.id
        )
      ).length ?? 0
    : 0;

  const totalStudents = review?.students.length ?? 0;

  /* =========================================
     TEACHER WORKFLOW
     KEEP EXISTING FUNCTIONALITY
  ========================================= */

  if (teacher) {
    return (
      <main className="result-workspace">
        <h1>Subject Results</h1>

        <p>
          Save a score for every enrolled student, then
          submit the full subject batch. Submitted scores
          remain locked until an administrator rejects the
          batch.
        </p>

        <Link to="/teacher/dashboard">
          Teacher dashboard
        </Link>

        {error && <p role="alert">{error}</p>}
        {message && <p role="status">{message}</p>}

        <fieldset
          disabled={busy}
          className="result-card result-filters"
        >
          <label>
            Academic session
            <select
              value={sessionID}
              onChange={(event) => {
                setSessionID(event.target.value);
                setClassID("");
                setSubjectID("");
                setTermID("");
                reset();
              }}
            >
              <option value="">Select session</option>

              {options.sessions.map((session) => (
                <option
                  key={session.id}
                  value={session.id}
                >
                  {session.name}
                </option>
              ))}
            </select>
          </label>

          <label>
            Assigned class and subject
            <select
              value={
                classID && subjectID
                  ? `${classID}/${subjectID}`
                  : ""
              }
              onChange={(event) => {
                const [selectedClass, selectedSubject] =
                  event.target.value.split("/");

                setClassID(selectedClass ?? "");
                setSubjectID(selectedSubject ?? "");
                reset();
              }}
            >
              <option value="">Select assignment</option>

              {assignments.map((assignment) => (
                <option
                  key={`${assignment.class_id}/${assignment.subject_id}`}
                  value={`${assignment.class_id}/${assignment.subject_id}`}
                >
                  {assignment.class_name} ·{" "}
                  {assignment.subject_name}
                </option>
              ))}
            </select>
          </label>

          <label>
            Term
            <select
              value={termID}
              onChange={(event) => {
                setTermID(event.target.value);
                setClassID("");
                setSubjectID("");
                reset();
              }}
            >
              <option value="">Select term</option>

              {availableTerms.map((term) => (
                <option key={term.id} value={term.id}>
                  {term.name.replaceAll("_", " ")}
                </option>
              ))}
            </select>
          </label>

          <button
            type="button"
            disabled={
              !classID ||
              !sessionID ||
              !termID ||
              !subjectID
            }
            onClick={() => void run(load)}
          >
            Load results
          </button>
        </fieldset>

        {review && (
          <>
            {review.publication && (
              <p role="status">
                Published{" "}
                {formatDateTime(
                  review.publication.published_at
                )}. This class-term is locked.
              </p>
            )}

            {review.batches.length === 0 && (
              <p role="alert">
                No subject offering configuration is
                available.
              </p>
            )}

            {review.batches.map((batch) => (
              <section
                className="result-card"
                key={batch.subject_id}
              >
                <h2>
                  {batch.subject_name}{" "}
                  <span className="result-status">
                    {batch.status}
                  </span>
                </h2>

                {batch.submitted_at && (
                  <p>
                    Submitted{" "}
                    {formatDateTime(batch.submitted_at)}
                  </p>
                )}

                {batch.reviewed_at && (
                  <p>
                    Reviewed{" "}
                    {formatDateTime(batch.reviewed_at)}
                  </p>
                )}

                {batch.rejection_reason && (
                  <p role="alert">
                    Rejection: {batch.rejection_reason}
                  </p>
                )}
              </section>
            ))}

            {review.batches.length > 0 && (
              <section className="result-card">
                <h2>Enrolled Student Scores</h2>

                {review.students.length === 0 && (
                  <p>No enrolled students.</p>
                )}

                {review.students.map((student) => {
                  const score = review.results.find(
                    (result) =>
                      result.student.id === student.id
                  );

                  return (
                    <ScoreForm
                      key={`${student.id}/${score?.ca_score}/${score?.exam_score}`}
                      student={student}
                      score={score}
                      disabled={busy || !editable}
                      save={(ca, exam) =>
                        save(student, score, ca, exam)
                      }
                    />
                  );
                })}

                <button
                  type="button"
                  disabled={
                    busy ||
                    !editable ||
                    !review.students.length ||
                    review.students.some(
                      (student) =>
                        !review.results.some(
                          (score) =>
                            score.student.id === student.id
                        )
                    )
                  }
                  onClick={() => void transition("submit")}
                >
                  {review.batches[0]?.status === "rejected"
                    ? "Resubmit full batch"
                    : "Submit full batch"}
                </button>
              </section>
            )}

            <section className="result-card">
              <h2>Batch History</h2>

              <ul>
                {review.events.map((event, index) => (
                  <li key={index}>
                    {review.batches.find(
                      (batch) =>
                        batch.subject_id ===
                        event.subject_id
                    )?.subject_name ?? "Subject"}
                    {" · "}
                    {event.action}
                    {" · "}
                    {formatDateTime(event.occurred_at)}
                    {" · actor "}
                    {event.actor_id}
                    {event.reason
                      ? ` · ${event.reason}`
                      : ""}
                  </li>
                ))}
              </ul>

              {review.events.length === 0 && (
                <p>No submissions or reviews yet.</p>
              )}
            </section>
          </>
        )}
      </main>
    );
  }

  /* =========================================
     ADMIN RESULTS MANAGEMENT
  ========================================= */

  return (
    <main className="result-workspace result-admin-page">
      {/* PAGE BANNER */}

      <section className="result-admin-banner">
        <div>
          <h1>Results Management</h1>
          <p>
            Review teacher-submitted results, track
            approvals, and publish completed term results.
          </p>
        </div>

        <GraduationCap
          size={94}
          className="result-admin-banner-icon"
          aria-hidden="true"
        />
      </section>

      {/* SUMMARY CARDS */}

      <section className="result-admin-stats">
        <div className="result-admin-stat-card">
          <div className="result-admin-stat-icon blue">
            <Layers size={27} />
          </div>

          <div>
            <span>Total Subject Batches</span>
            <strong>
              {review ? totalBatches : "—"}
            </strong>
            <small>Selected class-term</small>
          </div>
        </div>

        <div className="result-admin-stat-card">
          <div className="result-admin-stat-icon orange">
            <Clock3 size={27} />
          </div>

          <div>
            <span>Submitted</span>
            <strong>
              {review ? submittedCount : "—"}
            </strong>
            <small>Awaiting review</small>
          </div>
        </div>

        <div className="result-admin-stat-card">
          <div className="result-admin-stat-icon green">
            <CheckCircle2 size={27} />
          </div>

          <div>
            <span>Approved</span>
            <strong>
              {review ? approvedCount : "—"}
            </strong>
            <small>Reviewed and accepted</small>
          </div>
        </div>

        <div className="result-admin-stat-card">
          <div className="result-admin-stat-icon red">
            <XCircle size={27} />
          </div>

          <div>
            <span>Rejected</span>
            <strong>
              {review ? rejectedCount : "—"}
            </strong>
            <small>Awaiting corrections</small>
          </div>
        </div>
      </section>

      {/* ACADEMIC PERIOD FILTERS */}

      <section className="result-admin-panel">
        <div className="result-admin-panel-heading">
          <div>
            <h2>Academic Period</h2>
            <p>
              Select a session, class, and term to load
              subject results.
            </p>
          </div>

          <Link
            className="result-admin-subject-link"
            to="/admin/subjects"
          >
            <BookOpen size={17} />
            Manage Class Subjects
          </Link>
        </div>

        <fieldset
          disabled={busy || loadingOptions}
          className="result-admin-period-filters"
        >
          <label>
            Academic Session
            <select
              value={sessionID}
              onChange={(event) => {
                setSessionID(event.target.value);
                setClassID("");
                setTermID("");
                reset();
              }}
            >
              <option value="">Select session</option>

              {options.sessions.map((session) => (
                <option
                  key={session.id}
                  value={session.id}
                >
                  {session.name}
                </option>
              ))}
            </select>
          </label>

          <label>
            Class
            <select
              value={classID}
              onChange={(event) => {
                setClassID(event.target.value);
                reset();
              }}
            >
              <option value="">Select class</option>

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

          <label>
            Term
            <select
              value={termID}
              onChange={(event) => {
                setTermID(event.target.value);
                reset();
              }}
            >
              <option value="">Select term</option>

              {availableTerms.map((term) => (
                <option key={term.id} value={term.id}>
                  {term.name.replaceAll("_", " ")}
                </option>
              ))}
            </select>
          </label>

          <button
            type="button"
            className="result-admin-primary-button"
            disabled={
              !sessionID ||
              !classID ||
              !termID ||
              busy ||
              loadingOptions
            }
            onClick={() => void run(load)}
          >
            <Search size={17} />
            Load Results
          </button>
        </fieldset>
      </section>

      {/* GENERAL FEEDBACK */}

      {error && (
        <p className="result-admin-error" role="alert">
          {error}
        </p>
      )}

      {message && (
        <p className="result-admin-success" role="status">
          {message}
        </p>
      )}

      {/* RESULTS CONTENT */}

      {!review ? (
        <section className="result-admin-panel result-admin-placeholder">
          <ClipboardList size={42} />

          <h2>No Results Loaded</h2>

          <p>
            Select an academic session, class, and term,
            then click Load Results to review teacher
            submissions.
          </p>
        </section>
      ) : (
        <>
          {/* PUBLICATION STATUS */}

          <section className="result-admin-publication">
            <div>
              <span
                className={`result-admin-publication-badge ${
                  review.publication
                    ? "published"
                    : "unpublished"
                }`}
              >
                {review.publication
                  ? "Published"
                  : "Not Published"}
              </span>

              <h3>
                {selectedClass?.name ?? "Selected Class"}
                {" · "}
                {selectedSession?.name ?? "Academic Session"}
                {" · "}
                {selectedTerm?.name.replaceAll("_", " ") ??
                  "Term"}
              </h3>

              <p>
                {review.publication
                  ? `Published on ${formatDateTime(
                      review.publication.published_at
                    )}. This class-term is locked.`
                  : `${approvedCount} of ${totalBatches} subject batches approved.`}
              </p>
            </div>

            <button
              type="button"
              className="result-admin-publish-button"
              disabled={busy || !canPublish}
              onClick={() => void publishResults()}
              title={
                canPublish
                  ? "Publish the complete class-term"
                  : "Every offered subject must be approved before publication"
              }
            >
              <Send size={17} />
              {review.publication
                ? "Already Published"
                : "Publish Complete Class-Term"}
            </button>
          </section>

          {review.batches.length === 0 && (
            <p className="result-admin-error" role="alert">
              No subject offering configuration is
              available for this class-term. Configure
              the class subjects before continuing.
            </p>
          )}

          {/* OVERSIGHT AND PREVIEW */}

          <div className="result-admin-content-grid">
            {/* LEFT: OVERSIGHT TABLE */}

            <section className="result-admin-panel result-admin-oversight">
              <div className="result-admin-panel-heading">
                <div>
                  <h2>Result Oversight</h2>
                  <p>
                    Review subject batch submissions
                    and approval status.
                  </p>
                </div>
              </div>

              <div className="result-admin-toolbar">
                <div className="result-admin-search">
                  <Search size={17} />

                  <input
                    type="search"
                    placeholder="Search subjects..."
                    aria-label="Search subjects"
                    value={search}
                    onChange={(event) => {
                      setSearch(event.target.value);
                      setCurrentPage(1);
                    }}
                  />
                </div>

                <select
                  aria-label="Filter by subject"
                  value={subjectFilter}
                  onChange={(event) => {
                    setSubjectFilter(event.target.value);
                    setCurrentPage(1);
                  }}
                >
                  <option value="all">All Subjects</option>

                  {allBatches.map((batch) => (
                    <option
                      key={batch.subject_id}
                      value={batch.subject_id}
                    >
                      {batch.subject_name}
                    </option>
                  ))}
                </select>

                <select
                  aria-label="Filter by status"
                  value={statusFilter}
                  onChange={(event) => {
                    setStatusFilter(event.target.value);
                    setCurrentPage(1);
                  }}
                >
                  <option value="all">All Statuses</option>
                  <option value="missing">Missing</option>
                  <option value="draft">Draft</option>
                  <option value="submitted">
                    Submitted
                  </option>
                  <option value="approved">Approved</option>
                  <option value="rejected">Rejected</option>
                </select>
              </div>

              <div className="result-table-scroll">
                <table className="result-admin-table">
                  <thead>
                    <tr>
                      <th>#</th>
                      <th>Subject</th>
                      <th>Coverage</th>
                      <th>Status</th>
                      <th>Submitted</th>
                      <th>Action</th>
                    </tr>
                  </thead>

                  <tbody>
                    {visibleBatches.length === 0 ? (
                      <tr>
                        <td
                          colSpan={6}
                          className="result-admin-empty"
                        >
                          No matching subject batches.
                        </td>
                      </tr>
                    ) : (
                      visibleBatches.map((batch, index) => {
                        const recordedScores =
                          review.results.filter(
                            (score) =>
                              score.subject.id ===
                              batch.subject_id
                          ).length;

                        return (
                          <tr
                            key={batch.subject_id}
                            className={
                              selectedBatchSubjectID ===
                              batch.subject_id
                                ? "result-admin-selected-row"
                                : ""
                            }
                          >
                            <td>
                              {(safePage - 1) *
                                PAGE_SIZE +
                                index +
                                1}
                            </td>

                            <td>
                              <strong>
                                {batch.subject_name}
                              </strong>
                            </td>

                            <td>
                              {recordedScores}/
                              {totalStudents}
                            </td>

                            <td>
                              <span
                                className={`result-admin-status ${batch.status}`}
                              >
                                {formatStatus(
                                  batch.status
                                )}
                              </span>
                            </td>

                            <td>
                              {formatDate(
                                batch.submitted_at
                              )}
                            </td>

                            <td>
                              <button
                                type="button"
                                className="result-admin-review-button"
                                onClick={() => {
                                  setSelectedBatchSubjectID(
                                    batch.subject_id
                                  );
                                  setError("");
                                }}
                              >
                                <Eye size={15} />
                                Review
                              </button>
                            </td>
                          </tr>
                        );
                      })
                    )}
                  </tbody>
                </table>
              </div>

              <div className="result-admin-pagination">
                <span>
                  Showing{" "}
                  {filteredBatches.length === 0
                    ? 0
                    : (safePage - 1) *
                        PAGE_SIZE +
                      1}
                  {" – "}
                  {Math.min(
                    safePage * PAGE_SIZE,
                    filteredBatches.length
                  )}
                  {" of "}
                  {filteredBatches.length}
                </span>

                <div>
                  <button
                    type="button"
                    disabled={safePage === 1}
                    onClick={() =>
                      setCurrentPage(safePage - 1)
                    }
                    aria-label="Previous page"
                  >
                    <ChevronLeft size={16} />
                  </button>

                  <span>
                    Page {safePage} of {totalPages}
                  </span>

                  <button
                    type="button"
                    disabled={
                      safePage === totalPages
                    }
                    onClick={() =>
                      setCurrentPage(safePage + 1)
                    }
                    aria-label="Next page"
                  >
                    <ChevronRight size={16} />
                  </button>
                </div>
              </div>
            </section>

            {/* RIGHT: SUBMISSION PREVIEW */}

            <section className="result-admin-panel result-admin-preview">
              <div className="result-admin-panel-heading">
                <div>
                  <h2>Submission Preview</h2>
                  <p>
                    View student scores and review
                    the selected subject batch.
                  </p>
                </div>

                <Eye size={20} />
              </div>

              {!selectedBatch ? (
                <div className="result-admin-preview-empty">
                  <FileCheck2 size={40} />

                  <p>
                    Select a subject batch to view
                    its results.
                  </p>
                </div>
              ) : (
                <>
                  <div className="result-admin-preview-header">
                    <div>
                      <span>Selected Subject</span>
                      <h3>
                        {selectedBatch.subject_name}
                      </h3>
                    </div>

                    <span
                      className={`result-admin-status ${selectedBatch.status}`}
                    >
                      {formatStatus(
                        selectedBatch.status
                      )}
                    </span>
                  </div>

                  <div className="result-admin-detail-grid">
                    <div>
                      <span>Student Scores</span>
                      <strong>
                        {studentsWithScores}/
                        {totalStudents}
                      </strong>
                    </div>

                    <div>
                      <span>Submitted At</span>
                      <strong>
                        {formatDateTime(
                          selectedBatch.submitted_at
                        )}
                      </strong>
                    </div>

                    <div>
                      <span>Reviewed At</span>
                      <strong>
                        {formatDateTime(
                          selectedBatch.reviewed_at
                        )}
                      </strong>
                    </div>

                    <div>
                      <span>Batch Status</span>
                      <strong>
                        {formatStatus(
                          selectedBatch.status
                        )}
                      </strong>
                    </div>
                  </div>

                  {selectedBatch.rejection_reason && (
                    <div className="result-admin-rejection-note">
                      <strong>
                        Previous Rejection Reason
                      </strong>

                      <p>
                        {selectedBatch.rejection_reason}
                      </p>
                    </div>
                  )}

                  {/* READ-ONLY SCORES */}

                  <div className="result-admin-scores-heading">
                    <h3>Student Scores</h3>
                    <span>Read-only</span>
                  </div>

                  <div className="result-table-scroll">
                    <table className="result-admin-score-table">
                      <thead>
                        <tr>
                          <th>Student</th>
                          <th>CA</th>
                          <th>Exam</th>
                          <th>Total</th>
                          <th>Grade</th>
                        </tr>
                      </thead>

                      <tbody>
                        {review.students.length === 0 ? (
                          <tr>
                            <td
                              colSpan={5}
                              className="result-admin-empty"
                            >
                              No enrolled students.
                            </td>
                          </tr>
                        ) : (
                          review.students.map(
                            (student) => {
                              const score =
                                selectedBatchScores.find(
                                  (result) =>
                                    result.student.id ===
                                    student.id
                                );

                              return (
                                <tr key={student.id}>
                                  <td>
                                    {student.name}
                                  </td>

                                  <td>
                                    {score?.ca_score ??
                                      "Missing"}
                                  </td>

                                  <td>
                                    {score?.exam_score ??
                                      "—"}
                                  </td>

                                  <td>
                                    {score?.total_score ??
                                      "—"}
                                  </td>

                                  <td>
                                    {score?.grade ?? "—"}
                                  </td>
                                </tr>
                              );
                            }
                          )
                        )}
                      </tbody>
                    </table>
                  </div>

                  {/* REVIEW ACTIONS */}

                  {selectedBatch.status === "submitted" &&
                    !review.publication && (
                      <fieldset
                        disabled={busy}
                        className="result-admin-review-actions"
                      >
                        <button
                          type="button"
                          className="result-admin-approve-button"
                          onClick={() =>
                            void transition(
                              "approve",
                              selectedBatch.subject_id
                            )
                          }
                        >
                          <CheckCircle2 size={17} />
                          Approve Batch
                        </button>

                        <form
                          onSubmit={(event) => {
                            event.preventDefault();

                            void transition(
                              "reject",
                              selectedBatch.subject_id
                            );
                          }}
                        >
                          <label>
                            Rejection Reason
                            <textarea
                              required
                              maxLength={2000}
                              placeholder="Explain why the batch is being rejected..."
                              value={
                                reason[
                                  selectedBatch.subject_id
                                ] ?? ""
                              }
                              onChange={(event) =>
                                setReason({
                                  ...reason,
                                  [selectedBatch.subject_id]:
                                    event.target.value,
                                })
                              }
                            />
                          </label>

                          <button
                            type="submit"
                            className="result-admin-reject-button"
                          >
                            <XCircle size={17} />
                            Reject Batch
                          </button>
                        </form>
                      </fieldset>
                    )}

                  {review.publication && (
                    <p className="result-admin-info">
                      This class-term has been published
                      and is locked.
                    </p>
                  )}

                  {/* SELECTED BATCH HISTORY */}

                  <div className="result-admin-history">
                    <h3>Batch History</h3>

                    {selectedBatchHistory.length === 0 ? (
                      <p>
                        No submission or review events yet.
                      </p>
                    ) : (
                      <ul>
                        {selectedBatchHistory.map(
                          (event, index) => (
                            <li key={index}>
                              <strong>
                                {event.action}
                              </strong>

                              <span>
                                {formatDateTime(
                                  event.occurred_at
                                )}
                              </span>

                              <small>
                                Actor: {event.actor_id}
                              </small>

                              {event.reason && (
                                <p>{event.reason}</p>
                              )}
                            </li>
                          )
                        )}
                      </ul>
                    )}
                  </div>
                </>
              )}
            </section>
          </div>
        </>
      )}
    </main>
  );
}

export default ResultWorkflow;
