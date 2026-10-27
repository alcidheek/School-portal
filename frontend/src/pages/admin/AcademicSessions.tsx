
import { useEffect, useState } from "react";
import {
  CalendarDays,
  CheckCircle2,
  ChevronRight,
  CirclePlus,
  Clock3,
  GraduationCap,
  RefreshCw,
   Trash2,
  X,
} from "lucide-react";

import {
  getAcademicSessions,
  getAcademicTerms,
  createAcademicSession,
  activateAcademicSession,
  deleteAcademicSession,
  createOrActivateAcademicTerm,
  type AcademicSession,
  type AcademicTerm,
  type CreateAcademicSessionRequest,
  type CreateAcademicTermRequest,
} from "../../services/academicService";

import "./AcademicSessions.css";

type TermName = AcademicTerm["name"];
type DialogType = "session" | "term" | null;

const termNames: TermName[] = [
  "FIRST_TERM",
  "SECOND_TERM",
  "THIRD_TERM",
];

const initialSessionForm: CreateAcademicSessionRequest = {
  name: "",
  start_date: "",
  end_date: "",
  activate: false,
  term_name: "FIRST_TERM",
  term_start_date: "",
  term_end_date: "",
};

const initialTermForm: CreateAcademicTermRequest = {
  term_name: "SECOND_TERM",
  start_date: "",
  end_date: "",
};

function formatDate(date: string | undefined): string {
  if (!date) return "—";

  const parsedDate = new Date(date);

  if (Number.isNaN(parsedDate.getTime())) {
    return date;
  }

  return parsedDate.toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    timeZone: "UTC",
  });
}

function formatTermName(name: TermName): string {
  return name
    .replaceAll("_", " ")
    .toLowerCase()
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function getErrorMessage(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "Something went wrong. Please try again.";
}

function datesAreValid(startDate: string, endDate: string) {
  return Boolean(startDate && endDate && startDate < endDate);
}

 function AcademicSessions() {
  const [sessions, setSessions] = useState<AcademicSession[]>([]);
  const [terms, setTerms] = useState<AcademicTerm[]>([]);

  const [selectedSessionID, setSelectedSessionID] = useState("");
  const [activeTerm, setActiveTerm] = useState<AcademicTerm | null>(null);

  const [loading, setLoading] = useState(true);
  const [termsLoading, setTermsLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const [dialog, setDialog] = useState<DialogType>(null);

  const [sessionForm, setSessionForm] =
    useState<CreateAcademicSessionRequest>(initialSessionForm);

  const [termForm, setTermForm] =
    useState<CreateAcademicTermRequest>(initialTermForm);

  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const selectedSession =
    sessions.find((session) => session.id === selectedSessionID) ?? null;

  const currentSession =
    sessions.find((session) => session.is_active) ?? null;

  const activeSessionCount = sessions.filter(
    (session) => session.is_active
  ).length;

  async function loadSessions(preferredSessionID?: string) {
    const sessionRecords = await getAcademicSessions();

    setSessions(sessionRecords);

    setSelectedSessionID((previousSessionID) => {
      if (
        preferredSessionID &&
        sessionRecords.some((session) => session.id === preferredSessionID)
      ) {
        return preferredSessionID;
      }

      if (
        previousSessionID &&
        sessionRecords.some((session) => session.id === previousSessionID)
      ) {
        return previousSessionID;
      }

      return (
        sessionRecords.find((session) => session.is_active)?.id ??
        sessionRecords[0]?.id ??
        ""
      );
    });

    const activeSession = sessionRecords.find(
      (session) => session.is_active
    );

    if (!activeSession) {
      setActiveTerm(null);
      return;
    }

    const sessionTerms = await getAcademicTerms(activeSession.id);

    setActiveTerm(
      sessionTerms.find((term) => term.is_active) ?? null
    );
  }

  useEffect(() => {
    let cancelled = false;

    async function initializePage() {
      try {
        const sessionRecords = await getAcademicSessions();

        if (cancelled) return;

        setSessions(sessionRecords);

        const currentSession = sessionRecords.find(
          (session) => session.is_active
        );

        setSelectedSessionID(
          currentSession?.id ?? sessionRecords[0]?.id ?? ""
        );

        if (currentSession) {
          const sessionTerms = await getAcademicTerms(currentSession.id);

          if (!cancelled) {
            setActiveTerm(
              sessionTerms.find((term) => term.is_active) ?? null
            );
          }
        }
      } catch (error) {
        if (!cancelled) {
          setError(getErrorMessage(error));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void initializePage();

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function loadSelectedTerms() {
      if (!selectedSessionID) {
        setTerms([]);
        return;
      }

      setTerms([]);
      setTermsLoading(true);

      try {
        const sessionTerms = await getAcademicTerms(selectedSessionID);

        if (!cancelled) {
          setTerms(sessionTerms);
        }
      } catch (error) {
        if (!cancelled) {
          setError(getErrorMessage(error));
        }
      } finally {
        if (!cancelled) {
          setTermsLoading(false);
        }
      }
    }

    void loadSelectedTerms();

    return () => {
      cancelled = true;
    };
  }, [selectedSessionID]);

  function openSessionDialog() {
    setSessionForm({ ...initialSessionForm });
    setError("");
    setDialog("session");
  }

  function openTermDialog() {
    const firstMissingTerm = termNames.find(
      (termName) => !terms.some((term) => term.name === termName)
    );

    setTermForm({
      ...initialTermForm,
      term_name: firstMissingTerm ?? "FIRST_TERM",
    });

    setError("");
    setDialog("term");
  }

  function closeDialog() {
    if (saving) return;

    setDialog(null);
    setError("");
  }

  async function handleCreateSession(
    event: React.FormEvent<HTMLFormElement>
  ) {
    event.preventDefault();

    if (!datesAreValid(sessionForm.start_date, sessionForm.end_date)) {
      setError("Session end date must be after its start date.");
      return;
    }

    if (
      !datesAreValid(
        sessionForm.term_start_date,
        sessionForm.term_end_date
      )
    ) {
      setError("Term end date must be after its start date.");
      return;
    }

    if (
      sessionForm.term_start_date < sessionForm.start_date ||
      sessionForm.term_end_date > sessionForm.end_date
    ) {
      setError("The initial term must fall within the session dates.");
      return;
    }

    setSaving(true);
    setError("");
    setMessage("");

    try {
      const createdSession = await createAcademicSession(sessionForm);

      await loadSessions(createdSession.id);

      setDialog(null);
      setMessage(`Academic session ${createdSession.name} created successfully.`);
    } catch (error) {
      setError(getErrorMessage(error));
    } finally {
      setSaving(false);
    }
  }

  async function handleActivateSession(session: AcademicSession) {
    const confirmed = window.confirm(
      `Activate academic session ${session.name}?\n\n` +
        "This will deactivate the currently active session and its term."
    );

    if (!confirmed) return;

    setSaving(true);
    setError("");
    setMessage("");

    try {
      await activateAcademicSession(session.id);

      await loadSessions(session.id);

      setMessage(`${session.name} is now the active academic session.`);
    } catch (error) {
      setError(getErrorMessage(error));
    } finally {
      setSaving(false);
    }
  }


async function handleDeleteSession(session: AcademicSession) {
  if (session.is_active) {
    setError("The active academic session cannot be deleted.");
    return;
  }

  const confirmed = window.confirm(
    `Are you sure you want to delete ${session.name}?\n\n` +
      "This action cannot be undone. Sessions containing protected academic records cannot be deleted."
  );

  if (!confirmed) return;

  setSaving(true);
  setError("");
  setMessage("");

  try {
    await deleteAcademicSession(session.id);

    await loadSessions();

    if (selectedSessionID === session.id) {
      setTerms([]);
    }

    setMessage(
      `Academic session ${session.name} deleted successfully.`
    );
  } catch (error) {
    setError(getErrorMessage(error));
  } finally {
    setSaving(false);
  }
}


  async function handleSaveTerm(
    event: React.FormEvent<HTMLFormElement>
  ) {
    event.preventDefault();

    if (!selectedSession || !selectedSession.is_active) {
      setError("Select an active academic session first.");
      return;
    }

    if (!datesAreValid(termForm.start_date, termForm.end_date)) {
      setError("Term end date must be after its start date.");
      return;
    }

    const sessionStart = selectedSession.start_date.slice(0, 10);
    const sessionEnd = selectedSession.end_date.slice(0, 10);

    if (
      termForm.start_date < sessionStart ||
      termForm.end_date > sessionEnd
    ) {
      setError("Term dates must fall within the academic session.");
      return;
    }

    const confirmed = window.confirm(
      `Create and activate ${formatTermName(termForm.term_name)}?\n\n` +
        "Creating a term will make it the active term."
    );

    if (!confirmed) return;

    setSaving(true);
    setError("");
    setMessage("");

    try {
      await createOrActivateAcademicTerm(
        selectedSession.id,
        termForm
      );

      const updatedTerms = await getAcademicTerms(selectedSession.id);

      setTerms(updatedTerms);
      setActiveTerm(
        updatedTerms.find((term) => term.is_active) ?? null
      );

      setDialog(null);
      setMessage(
        `${formatTermName(termForm.term_name)} created and activated successfully.`
      );
    } catch (error) {
      setError(getErrorMessage(error));
    } finally {
      setSaving(false);
    }
  }

  async function handleActivateTerm(term: AcademicTerm) {
    if (!selectedSession?.is_active) {
      setError("You must activate the academic session first.");
      return;
    }

    const confirmed = window.confirm(
      `Activate ${formatTermName(term.name)} for ${selectedSession.name}?\n\n` +
        "The current active term will be deactivated."
    );

    if (!confirmed) return;

    setSaving(true);
    setError("");
    setMessage("");

    try {
      await createOrActivateAcademicTerm(selectedSession.id, {
        term_name: term.name,
        start_date: term.start_date.slice(0, 10),
        end_date: term.end_date.slice(0, 10),
      });

      const updatedTerms = await getAcademicTerms(selectedSession.id);

      setTerms(updatedTerms);
      setActiveTerm(
        updatedTerms.find((item) => item.is_active) ?? null
      );

      setMessage(`${formatTermName(term.name)} activated successfully.`);
    } catch (error) {
      setError(getErrorMessage(error));
    } finally {
      setSaving(false);
    }
  }

  async function handleRefresh() {
    setLoading(true);
    setError("");
    setMessage("");

    try {
      await loadSessions();

      if (selectedSessionID) {
        const updatedTerms = await getAcademicTerms(selectedSessionID);
        setTerms(updatedTerms);
      }
    } catch (error) {
      setError(getErrorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="academic-page">
      <section className="academic-banner">
        <div className="academic-banner-content">
          <div className="academic-banner-icon">
            <GraduationCap size={24} />
          </div>

          <h1>Academic Sessions &amp; Terms</h1>

          <p>
            Manage academic sessions, term schedules, and active
            academic periods.
          </p>
        </div>

        <CalendarDays
          size={110}
          className="academic-banner-decoration"
        />
      </section>

      {message && (
        <div className="academic-message academic-message-success" role="status">
          <CheckCircle2 size={18} />
          {message}
        </div>
      )}

      {error && (
        <div className="academic-message academic-message-error" role="alert">
          {error}
          <button type="button" onClick={() => setError("")}>
            <X size={16} />
          </button>
        </div>
      )}

      <div className="academic-section-heading">
        <div>
          <h2>Academic Overview</h2>
          <p>Current academic period at a glance</p>
        </div>

        <button
          className="academic-secondary-button"
          onClick={() => void handleRefresh()}
          disabled={loading || saving}
        >
          <RefreshCw size={16} />
          Refresh
        </button>
      </div>

      <section className="academic-stat-grid">
        <div className="academic-stat-card">
          <div className="academic-stat-icon">
            <CalendarDays size={22} />
          </div>

          <span>Current Academic Session</span>
          <strong>{currentSession?.name ?? "No Active Session"}</strong>
          <small>{currentSession ? "Active session" : "Not configured"}</small>
        </div>

        <div className="academic-stat-card">
          <div className="academic-stat-icon">
            <Clock3 size={22} />
          </div>

          <span>Current Term</span>
          <strong>
            {activeTerm ? formatTermName(activeTerm.name) : "No Active Term"}
          </strong>
          <small>
            {activeTerm
              ? `${formatDate(activeTerm.start_date)} – ${formatDate(activeTerm.end_date)}`
              : "Not configured"}
          </small>
        </div>

        <div className="academic-stat-card">
          <div className="academic-stat-icon">
            <GraduationCap size={22} />
          </div>

          <span>Total Academic Sessions</span>
          <strong>{sessions.length}</strong>
          <small>Sessions recorded</small>
        </div>

        <div className="academic-stat-card">
          <div className="academic-stat-icon">
            <CheckCircle2 size={22} />
          </div>

          <span>Active Sessions</span>
          <strong>{activeSessionCount}</strong>
          <small>Currently active</small>
        </div>
      </section>

      <section className="academic-panel">
        <div className="academic-panel-heading">
          <div>
            <h2>Academic Sessions</h2>
            <p>View and manage your school's academic sessions</p>
          </div>

          <button
            className="academic-primary-button"
            onClick={openSessionDialog}
            disabled={saving}
          >
            <CirclePlus size={17} />
            Create Session
          </button>
        </div>

        <div className="academic-table-container">
          <table className="academic-table">
            <thead>
              <tr>
                <th>Academic Session</th>
                <th>Start Date</th>
                <th>End Date</th>
                <th>Status</th>
                <th>Actions</th>
              </tr>
            </thead>

            <tbody>
              {sessions.map((session) => (
                <tr key={session.id}>
                  <td>
                    <strong>{session.name}</strong>
                  </td>

                  <td>{formatDate(session.start_date)}</td>
                  <td>{formatDate(session.end_date)}</td>

                  <td>
                    <span
                      className={
                        session.is_active
                          ? "academic-status academic-status-active"
                          : "academic-status academic-status-inactive"
                      }
                    >
                      {session.is_active ? "Active" : "Inactive"}
                    </span>
                  </td>

                  <td>
                    <div className="academic-table-actions">
                      <button
                        className="academic-text-button"
                        onClick={() => setSelectedSessionID(session.id)}
                      >
                        View Terms
                        <ChevronRight size={15} />
                      </button>


                    {!session.is_active && (
               <>
                        <button
                         type="button"
                          className="academic-outline-button"
                           disabled={saving}
                           onClick={() => void handleActivateSession(session)}
                          >
                      Activate
                     </button>

                     <button
                      type="button"
                      className="academic-delete-button"
                      disabled={saving}
                       onClick={() => void handleDeleteSession(session)}
                        title={`Delete ${session.name}`}
    >
      <Trash2 size={15} />
      Delete
    </button>
  </>
)}


                    </div>
                  </td>
                </tr>
              ))}

              {!loading && sessions.length === 0 && (
                <tr>
                  <td colSpan={5} className="academic-empty-table">
                    No academic sessions have been created yet.
                  </td>
                </tr>
              )}

              {loading && sessions.length === 0 && (
                <tr>
                  <td colSpan={5} className="academic-empty-table">
                    Loading academic sessions...
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>

      <section className="academic-panel">
        <div className="academic-panel-heading">
          <div>
            <h2>Academic Terms</h2>
            <p>
              {selectedSession
                ? `Terms for ${selectedSession.name}`
                : "Select an academic session to view its terms"}
            </p>
          </div>

          <button
            className="academic-primary-button"
            onClick={openTermDialog}
            disabled={
              !selectedSession?.is_active ||
              termsLoading ||
              saving ||
              terms.length >= 3
            }
            title={
              !selectedSession?.is_active
                ? "Activate the session before creating a term"
                : "Create a new academic term"
            }
          >
            <CirclePlus size={17} />
            Create Term
          </button>
        </div>

        <div className="academic-session-selector">
          <label htmlFor="academic-selected-session">
            Selected academic session
          </label>

          <select
            id="academic-selected-session"
            value={selectedSessionID}
            onChange={(event) => setSelectedSessionID(event.target.value)}
          >
            {sessions.length === 0 && (
              <option value="">No sessions available</option>
            )}

            {sessions.map((session) => (
              <option key={session.id} value={session.id}>
                {session.name}
                {session.is_active ? " (Active)" : ""}
              </option>
            ))}
          </select>
        </div>

        <div className="academic-table-container">
          <table className="academic-table">
            <thead>
              <tr>
                <th>Term</th>
                <th>Start Date</th>
                <th>End Date</th>
                <th>Status</th>
                <th>Actions</th>
              </tr>
            </thead>

            <tbody>
              {termNames.map((termName) => {
                const term = terms.find(
                  (record) => record.name === termName
                );

                return (
                  <tr key={termName}>
                    <td>
                      <strong>{formatTermName(termName)}</strong>
                    </td>

                    <td>
                      {term ? formatDate(term.start_date) : "—"}
                    </td>

                    <td>
                      {term ? formatDate(term.end_date) : "—"}
                    </td>

                    <td>
                      <span
                        className={
                          !term
                            ? "academic-status academic-status-missing"
                            : term.is_active && selectedSession?.is_active
                              ? "academic-status academic-status-active"
                              : "academic-status academic-status-inactive"
                        }
                      >
                        {!term
                          ? "Not Created"
                          : term.is_active && selectedSession?.is_active
                            ? "Active"
                            : "Inactive"}
                      </span>
                    </td>

                    <td>
                      {term &&
                        !term.is_active &&
                        selectedSession?.is_active && (
                          <button
                            className="academic-outline-button"
                            disabled={saving}
                            onClick={() =>
                              void handleActivateTerm(term)
                            }
                          >
                            Activate
                          </button>
                        )}

                      {!term && (
                        <span className="academic-muted-text">—</span>
                      )}

                      {term &&
                        term.is_active &&
                        selectedSession?.is_active && (
                          <span className="academic-active-label">
                            <CheckCircle2 size={15} />
                            Current Term
                          </span>
                        )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        {termsLoading && (
          <p className="academic-loading-text">Loading term records...</p>
        )}

        {!selectedSession?.is_active && selectedSession && (
          <p className="academic-information">
            Activate this academic session before creating or switching terms.
          </p>
        )}
      </section>

      {dialog && (
        <div
          className="academic-modal-backdrop"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) {
              closeDialog();
            }
          }}
        >
          <div
            className="academic-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="academic-modal-title"
          >
            <div className="academic-modal-header">
              <div>
                <h2 id="academic-modal-title">
                  {dialog === "session"
                    ? "Create Academic Session"
                    : "Create Academic Term"}
                </h2>
                <p>
                  {dialog === "session"
                    ? "Enter the session and initial term details."
                    : `Add a term to ${selectedSession?.name ?? "the selected session"}.`}
                </p>
              </div>

              <button
                type="button"
                className="academic-close-button"
                disabled={saving}
                onClick={closeDialog}
                aria-label="Close dialog"
              >
                <X size={20} />
              </button>
            </div>

            {dialog === "session" ? (
              <form onSubmit={(event) => void handleCreateSession(event)}>
                <div className="academic-modal-body">
                  <label>
                    Academic Session Name
                    <input
                      required
                      placeholder="e.g. 2027/2028"
                      value={sessionForm.name}
                      onChange={(event) =>
                        setSessionForm({
                          ...sessionForm,
                          name: event.target.value,
                        })
                      }
                    />
                  </label>

                  <div className="academic-form-row">
                    <label>
                      Session Start Date
                      <input
                        type="date"
                        required
                        value={sessionForm.start_date}
                        onChange={(event) =>
                          setSessionForm({
                            ...sessionForm,
                            start_date: event.target.value,
                          })
                        }
                      />
                    </label>

                    <label>
                      Session End Date
                      <input
                        type="date"
                        required
                        value={sessionForm.end_date}
                        onChange={(event) =>
                          setSessionForm({
                            ...sessionForm,
                            end_date: event.target.value,
                          })
                        }
                      />
                    </label>
                  </div>

                  <h3>Initial Academic Term</h3>

                  <label>
                    Term
                    <select
                      value={sessionForm.term_name}
                      onChange={(event) =>
                        setSessionForm({
                          ...sessionForm,
                          term_name: event.target.value as TermName,
                        })
                      }
                    >
                      {termNames.map((termName) => (
                        <option key={termName} value={termName}>
                          {formatTermName(termName)}
                        </option>
                      ))}
                    </select>
                  </label>

                  <div className="academic-form-row">
                    <label>
                      Term Start Date
                      <input
                        type="date"
                        required
                        value={sessionForm.term_start_date}
                        onChange={(event) =>
                          setSessionForm({
                            ...sessionForm,
                            term_start_date: event.target.value,
                          })
                        }
                      />
                    </label>

                    <label>
                      Term End Date
                      <input
                        type="date"
                        required
                        value={sessionForm.term_end_date}
                        onChange={(event) =>
                          setSessionForm({
                            ...sessionForm,
                            term_end_date: event.target.value,
                          })
                        }
                      />
                    </label>
                  </div>

                  <label className="academic-checkbox-label">
                    <input
                      type="checkbox"
                      checked={sessionForm.activate}
                      onChange={(event) =>
                        setSessionForm({
                          ...sessionForm,
                          activate: event.target.checked,
                        })
                      }
                    />
                    Activate this session immediately
                  </label>
                </div>

                <div className="academic-modal-footer">
                  <button
                    type="button"
                    className="academic-secondary-button"
                    onClick={closeDialog}
                    disabled={saving}
                  >
                    Cancel
                  </button>

                  <button
                    type="submit"
                    className="academic-primary-button"
                    disabled={saving}
                  >
                    {saving ? "Creating..." : "Create Session"}
                  </button>
                </div>
              </form>
            ) : (
              <form onSubmit={(event) => void handleSaveTerm(event)}>
                <div className="academic-modal-body">
                  <label>
                    Academic Term
                    <select
                      value={termForm.term_name}
                      onChange={(event) =>
                        setTermForm({
                          ...termForm,
                          term_name: event.target.value as TermName,
                        })
                      }
                    >
                      {termNames
                        .filter(
                          (termName) =>
                            !terms.some((term) => term.name === termName)
                        )
                        .map((termName) => (
                          <option key={termName} value={termName}>
                            {formatTermName(termName)}
                          </option>
                        ))}
                    </select>
                  </label>

                  <div className="academic-form-row">
                    <label>
                      Start Date
                      <input
                        type="date"
                        required
                        value={termForm.start_date}
                        onChange={(event) =>
                          setTermForm({
                            ...termForm,
                            start_date: event.target.value,
                          })
                        }
                      />
                    </label>

                    <label>
                      End Date
                      <input
                        type="date"
                        required
                        value={termForm.end_date}
                        onChange={(event) =>
                          setTermForm({
                            ...termForm,
                            end_date: event.target.value,
                          })
                        }
                      />
                    </label>
                  </div>

                  <p className="academic-information">
                    Creating this term will also make it the active term.
                    Existing academic records will be preserved.
                  </p>
                </div>

                <div className="academic-modal-footer">
                  <button
                    type="button"
                    className="academic-secondary-button"
                    onClick={closeDialog}
                    disabled={saving}
                  >
                    Cancel
                  </button>

                  <button
                    type="submit"
                    className="academic-primary-button"
                    disabled={saving}
                  >
                    {saving ? "Saving..." : "Create & Activate Term"}
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}
    </main>
  );
}
export default AcademicSessions;
