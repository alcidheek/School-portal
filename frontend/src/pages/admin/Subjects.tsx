
import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
import {
  BookOpen,
  GraduationCap,
  Layers,
  Link2,
  MoreHorizontal,
  Pencil,
  Plus,
  Search,
  Trash2,
  X,
} from "lucide-react";

import {
  getClasses,
  getAcademicSessions,
  getActiveTerm,
  type SchoolClass,
  type AcademicSession,
  type AcademicTerm,
} from "../../services/adminService";

import {
  getAcademicTerms,
} from "../../services/academicService";

import {
  getSubjects,
  createSubject,
  updateSubject,
  deleteSubject,
  getClassSubjectOfferings,
  addClassSubjectOffering,
  removeClassSubjectOffering,
  type Subject,
} from "../../services/subjectService";

import "./Subjects.css";

type SubjectTab = "catalogue" | "assignments";
type SubjectDialog = "create" | "edit" | "delete" | null;

function formatDate(value: string): string {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) return "—";

  return date.toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    timeZone: "UTC",
  });
}

function getErrorMessage(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "An unexpected error occurred.";
}

function Subjects() {
  const [activeTab, setActiveTab] =
    useState<SubjectTab>("catalogue");

  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [classes, setClasses] = useState<SchoolClass[]>([]);
  const [sessions, setSessions] = useState<AcademicSession[]>([]);
  const [terms, setTerms] = useState<AcademicTerm[]>([]);

  const [activeTerm, setActiveTerm] =
    useState<AcademicTerm | null>(null);

  const [selectedClassID, setSelectedClassID] = useState("");
  const [selectedSessionID, setSelectedSessionID] = useState("");
  const [selectedTermID, setSelectedTermID] = useState("");

  const [offeredSubjectIDs, setOfferedSubjectIDs] =
    useState<string[]>([]);

  const [search, setSearch] = useState("");
  const [assignmentSearch, setAssignmentSearch] = useState("");

  const [loading, setLoading] = useState(true);
  const [loadingTerms, setLoadingTerms] = useState(false);
  const [loadingOfferings, setLoadingOfferings] =
    useState(false);

  const [saving, setSaving] = useState(false);

  const [dialog, setDialog] = useState<SubjectDialog>(null);
  const [selectedSubject, setSelectedSubject] =
    useState<Subject | null>(null);
  const [subjectName, setSubjectName] = useState("");

  const [openMenuID, setOpenMenuID] = useState<string | null>(
    null
  );

  const [page, setPage] = useState(1);
  const pageSize = 5;

  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const selectedSession = sessions.find(
    (session) => session.id === selectedSessionID
  );

  const selectedTerm = terms.find(
    (term) => term.id === selectedTermID
  );

  const selectedClass = classes.find(
    (schoolClass) => schoolClass.id === selectedClassID
  );

  const canEditOfferings =
    Boolean(selectedSession?.is_active) &&
    Boolean(selectedTerm?.is_active) &&
    activeTerm?.id === selectedTermID &&
    activeTerm?.academic_session_id === selectedSessionID;

  const filteredSubjects = useMemo(() => {
    const searchValue = search.trim().toLowerCase();

    return subjects.filter((subject) =>
      subject.name.toLowerCase().includes(searchValue)
    );
  }, [subjects, search]);

  const totalPages = Math.max(
    1,
    Math.ceil(filteredSubjects.length / pageSize)
  );

  const visibleSubjects = filteredSubjects.slice(
    (page - 1) * pageSize,
    page * pageSize
  );

  const filteredAssignmentSubjects = subjects.filter(
    (subject) =>
      subject.name
        .toLowerCase()
        .includes(assignmentSearch.trim().toLowerCase())
  );

  const offeredSubjectSet = new Set(offeredSubjectIDs);

  useEffect(() => {
    setPage(1);
    setOpenMenuID(null);
  }, [search]);

  useEffect(() => {
    let cancelled = false;

    async function loadData() {
      try {
      const [
  subjectRecords,
  classRecords,
  sessionRecords,
] = await Promise.all([
  getSubjects(),
  getClasses(),
  getAcademicSessions(),
]);



const currentSession = sessionRecords.find(
  (session) => session.is_active
);

const currentTerm = currentSession
  ? await getActiveTerm(currentSession.id)
  : null;

if (cancelled) return;

setSubjects(subjectRecords);
setClasses(classRecords);
setSessions(sessionRecords);
setActiveTerm(currentTerm);

setSelectedClassID(classRecords[0]?.id ?? "");
setSelectedSessionID(currentSession?.id ?? "");

      } catch (requestError) {
        if (!cancelled) {
          setError(getErrorMessage(requestError));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void loadData();

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    setTerms([]);
    setSelectedTermID("");
    setOfferedSubjectIDs([]);

    if (!selectedSessionID) return;

    async function loadTerms() {
      setLoadingTerms(true);

      try {
        const termRecords = await getAcademicTerms(
          selectedSessionID
        );

        if (cancelled) return;

        setTerms(termRecords);

        const currentTerm = termRecords.find(
          (term) => term.id === activeTerm?.id
        );

        setSelectedTermID(
          currentTerm?.id ?? termRecords[0]?.id ?? ""
        );
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
  }, [selectedSessionID, activeTerm?.id]);

  useEffect(() => {
    let cancelled = false;

    setOfferedSubjectIDs([]);

    if (
      !selectedClassID ||
      !selectedSessionID ||
      !selectedTermID
    ) {
      setLoadingOfferings(false);
      return;
    }

    async function loadOfferings() {
      setLoadingOfferings(true);

      try {
        const response = await getClassSubjectOfferings(
          selectedClassID,
          selectedSessionID,
          selectedTermID
        );

        if (!cancelled) {
          setOfferedSubjectIDs(
            response.subjects.map((subject) => subject.id)
          );
        }
      } catch (requestError) {
        if (!cancelled) {
          setError(getErrorMessage(requestError));
        }
      } finally {
        if (!cancelled) setLoadingOfferings(false);
      }
    }

    void loadOfferings();

    return () => {
      cancelled = true;
    };
  }, [selectedClassID, selectedSessionID, selectedTermID]);

  function openCreateDialog() {
    setSelectedSubject(null);
    setSubjectName("");
    setOpenMenuID(null);
    setError("");
    setDialog("create");
  }

  function openEditDialog(subject: Subject) {
    setSelectedSubject(subject);
    setSubjectName(subject.name);
    setOpenMenuID(null);
    setError("");
    setDialog("edit");
  }

  function openDeleteDialog(subject: Subject) {
    setSelectedSubject(subject);
    setOpenMenuID(null);
    setError("");
    setDialog("delete");
  }

  function closeDialog() {
    if (saving) return;

    setDialog(null);
    setSelectedSubject(null);
    setSubjectName("");
    setError("");
  }

  async function refreshSubjects() {
    const subjectRecords = await getSubjects();
    setSubjects(subjectRecords);
  }

  async function handleSaveSubject(event: FormEvent) {
    event.preventDefault();

    const name = subjectName.trim();

    if (!name) {
      setError("Subject name is required.");
      return;
    }

    if (saving) return;

    setSaving(true);
    setError("");
    setMessage("");

    try {
      if (dialog === "edit" && selectedSubject) {
        await updateSubject(selectedSubject.id, { name });
        setMessage("Subject updated successfully.");
      } else if (dialog === "create") {
        await createSubject({ name });
        setMessage("Subject created successfully.");
      } else {
        return;
      }

      await refreshSubjects();
      setDialog(null);
      setSelectedSubject(null);
      setSubjectName("");
    } catch (requestError) {
      setError(getErrorMessage(requestError));
    } finally {
      setSaving(false);
    }
  }

  async function handleDeleteSubject() {
    if (!selectedSubject || saving) return;

    setSaving(true);
    setError("");
    setMessage("");

    try {
      await deleteSubject(selectedSubject.id);
      await refreshSubjects();

      setMessage("Subject deleted successfully.");
      setDialog(null);
      setSelectedSubject(null);
    } catch (requestError) {
      setError(getErrorMessage(requestError));
    } finally {
      setSaving(false);
    }
  }

  async function handleToggleOffering(
    subject: Subject,
    currentlyOffered: boolean
  ) {
    if (
      !canEditOfferings ||
      loadingOfferings ||
      saving ||
      !selectedClassID ||
      !selectedSessionID ||
      !selectedTermID
    ) {
      return;
    }

    setSaving(true);
    setError("");
    setMessage("");

    try {
      const response = currentlyOffered
        ? await removeClassSubjectOffering(
            selectedClassID,
            subject.id,
            selectedSessionID,
            selectedTermID
          )
        : await addClassSubjectOffering(
            selectedClassID,
            subject.id,
            selectedSessionID,
            selectedTermID
          );

      setOfferedSubjectIDs(
        response.subjects.map((item) => item.id)
      );

      setMessage(
        currentlyOffered
          ? `${subject.name} removed from ${selectedClass?.name}.`
          : `${subject.name} assigned to ${selectedClass?.name}.`
      );
    } catch (requestError) {
      setError(getErrorMessage(requestError));
    } finally {
      setSaving(false);
    }
  }

  return (
    <main className="subjects-page">
      <header className="subjects-banner">
        <div>
          <h1>Subject Management</h1>
          <p>
            Manage school subjects and assign them to classes
            for academic sessions and terms.
          </p>
        </div>
        <BookOpen
          size={82}
          className="subjects-banner-icon"
          aria-hidden="true"
        />
      </header>

      <section className="subjects-stats">
        <div className="subjects-stat-card">
          <div className="subjects-stat-icon blue">
            <Layers size={27} />
          </div>
          <div>
            <span>Total Subjects</span>
            <strong>{loading ? "—" : subjects.length}</strong>
            <small>School subject catalogue</small>
          </div>
        </div>

        <div className="subjects-stat-card">
          <div className="subjects-stat-icon green">
            <BookOpen size={27} />
          </div>
          <div>
            <span>Offered Subjects</span>
            <strong>
              {loading || loadingOfferings
                ? "—"
                : selectedClassID && selectedTermID
                  ? offeredSubjectIDs.length
                  : "—"}
            </strong>
            <small>Selected class and term</small>
          </div>
        </div>

        <div className="subjects-stat-card">
          <div className="subjects-stat-icon purple">
            <GraduationCap size={27} />
          </div>
          <div>
            <span>Unassigned Subjects</span>
            <strong>
              {loading || loadingOfferings
                ? "—"
                : selectedClassID && selectedTermID
                  ? subjects.filter(
                      (subject) =>
                        !offeredSubjectSet.has(subject.id)
                    ).length
                  : "—"}
            </strong>
            <small>Not offered by selected class</small>
          </div>
        </div>

        <div className="subjects-stat-card">
          <div className="subjects-stat-icon orange">
            <Link2 size={27} />
          </div>
          <div>
            <span>Selected Class</span>
            <strong>{selectedClass?.name ?? "—"}</strong>
            <small>{selectedTerm?.name ?? "Choose a term"}</small>
          </div>
        </div>
      </section>

      <div className="subjects-tabs" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "catalogue"}
          className={
            activeTab === "catalogue" ? "active" : ""
          }
          onClick={() => {
            setActiveTab("catalogue");
            setError("");
            setMessage("");
          }}
        >
          <Layers size={17} />
          Subject Catalogue
        </button>

        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "assignments"}
          className={
            activeTab === "assignments" ? "active" : ""
          }
          onClick={() => {
            setActiveTab("assignments");
            setError("");
            setMessage("");
          }}
        >
          <Link2 size={17} />
          Class Subject Assignments
        </button>
      </div>

      <section className="subjects-panel">
        {activeTab === "catalogue" ? (
          <>
            <div className="subjects-panel-header">
              <div>
                <h2>Subject Catalogue</h2>
                <p>
                  View, create, edit and manage all school
                  subjects.
                </p>
              </div>

              <div className="subjects-panel-controls">
                <div className="subjects-search">
                  <Search size={17} />
                  <input
                    type="search"
                    placeholder="Search subjects..."
                    aria-label="Search subjects"
                    value={search}
                    onChange={(event) =>
                      setSearch(event.target.value)
                    }
                  />
                </div>

                <button
                  type="button"
                  className="subjects-primary-button"
                  onClick={openCreateDialog}
                >
                  <Plus size={18} />
                  Add Subject
                </button>
              </div>
            </div>

            {message && (
              <p className="subjects-success" role="status">
                {message}
              </p>
            )}

            {error && !dialog && (
              <p className="subjects-error" role="alert">
                {error}
              </p>
            )}

            <div className="subjects-table-wrapper">
              <table className="subjects-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>Subject Name</th>
                    <th>Created At</th>
                    <th>Updated At</th>
                    <th>Actions</th>
                  </tr>
                </thead>

                <tbody>
                  {loading ? (
                    <tr>
                      <td colSpan={5} className="subjects-empty">
                        Loading subjects...
                      </td>
                    </tr>
                  ) : visibleSubjects.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="subjects-empty">
                        {search
                          ? "No matching subjects."
                          : "No subjects have been created."}
                      </td>
                    </tr>
                  ) : (
                    visibleSubjects.map((subject, index) => (
                      <tr key={subject.id}>
                        <td>
                          {(page - 1) * pageSize + index + 1}
                        </td>
                        <td>
                          <strong>{subject.name}</strong>
                        </td>
                        <td>
                          {formatDate(subject.created_at)}
                        </td>
                        <td>
                          {formatDate(subject.updated_at)}
                        </td>
                        <td>
                          <div className="subjects-action">
                            <button
                              type="button"
                              className="subjects-menu-trigger"
                              aria-label={`Actions for ${subject.name}`}
                              aria-expanded={
                                openMenuID === subject.id
                              }
                              onClick={() =>
                                setOpenMenuID(
                                  openMenuID === subject.id
                                    ? null
                                    : subject.id
                                )
                              }
                            >
                              <MoreHorizontal size={19} />
                            </button>

                            {openMenuID === subject.id && (
                              <div className="subjects-action-menu">
                                <button
                                  type="button"
                                  onClick={() =>
                                    openEditDialog(subject)
                                  }
                                >
                                  <Pencil size={15} />
                                  Edit Subject
                                </button>
                                <button
                                  type="button"
                                  className="danger"
                                  onClick={() =>
                                    openDeleteDialog(subject)
                                  }
                                >
                                  <Trash2 size={15} />
                                  Delete Subject
                                </button>
                              </div>
                            )}
                          </div>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>

            <div className="subjects-pagination">
              <span>
                Showing{" "}
                {filteredSubjects.length === 0
                  ? 0
                  : (page - 1) * pageSize + 1}{" "}
                to{" "}
                {Math.min(
                  page * pageSize,
                  filteredSubjects.length
                )}{" "}
                of {filteredSubjects.length} subjects
              </span>

              <div>
                <button
                  type="button"
                  disabled={page === 1}
                  onClick={() => setPage(page - 1)}
                  aria-label="Previous page"
                >
                  ‹
                </button>

                {Array.from(
                  { length: totalPages },
                  (_, index) => index + 1
                ).map((pageNumber) => (
                  <button
                    key={pageNumber}
                    type="button"
                    className={
                      page === pageNumber ? "active" : ""
                    }
                    onClick={() => setPage(pageNumber)}
                  >
                    {pageNumber}
                  </button>
                ))}

                <button
                  type="button"
                  disabled={page >= totalPages}
                  onClick={() => setPage(page + 1)}
                  aria-label="Next page"
                >
                  ›
                </button>
              </div>
            </div>
          </>
        ) : (
          <>
            <div className="subjects-panel-header">
              <div>
                <h2>Class Subject Assignments</h2>
                <p>
                  Assign subjects to a class for a specific
                  academic session and term.
                </p>
              </div>
            </div>

            <div className="subjects-filters">
              <label>
                Academic Session
                <select
                  value={selectedSessionID}
                  onChange={(event) =>
                    setSelectedSessionID(event.target.value)
                  }
                >
                  <option value="">Select session</option>
                  {sessions.map((session) => (
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
                Academic Term
                <select
                  value={selectedTermID}
                  disabled={loadingTerms || !selectedSessionID}
                  onChange={(event) =>
                    setSelectedTermID(event.target.value)
                  }
                >
                  <option value="">
                    {loadingTerms
                      ? "Loading terms..."
                      : "Select term"}
                  </option>
                  {terms.map((term) => (
                    <option key={term.id} value={term.id}>
                      {term.name.replaceAll("_", " ")}
                    </option>
                  ))}
                </select>
              </label>

              <label>
                School Class
                <select
                  value={selectedClassID}
                  onChange={(event) =>
                    setSelectedClassID(event.target.value)
                  }
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
            </div>

            {!canEditOfferings &&
              selectedClassID &&
              selectedTermID && (
                <p className="subjects-info">
                  Historical or inactive academic period:
                  assignments are read-only.
                </p>
              )}

            {message && (
              <p className="subjects-success" role="status">
                {message}
              </p>
            )}

            {error && (
              <p className="subjects-error" role="alert">
                {error}
              </p>
            )}

            <div className="subjects-assignment-heading">
              <div>
                <h3>
                  Subjects for{" "}
                  {selectedClass?.name ?? "Selected Class"}
                </h3>
                <span>
                  {loadingOfferings
                    ? "Loading offerings..."
                    : `${offeredSubjectIDs.length} subjects assigned`}
                </span>
              </div>

              <div className="subjects-search">
                <Search size={17} />
                <input
                  type="search"
                  placeholder="Search available subjects..."
                  aria-label="Search available subjects"
                  value={assignmentSearch}
                  onChange={(event) =>
                    setAssignmentSearch(event.target.value)
                  }
                />
              </div>
            </div>

            <div className="subjects-table-wrapper">
              <table className="subjects-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>Subject Name</th>
                    <th>Status</th>
                    <th>Action</th>
                  </tr>
                </thead>

                <tbody>
                  {loading || loadingOfferings ? (
                    <tr>
                      <td
                        colSpan={4}
                        className="subjects-empty"
                      >
                        Loading assignments...
                      </td>
                    </tr>
                  ) : !selectedClassID ||
                    !selectedSessionID ||
                    !selectedTermID ? (
                    <tr>
                      <td
                        colSpan={4}
                        className="subjects-empty"
                      >
                        Select a class, session and term.
                      </td>
                    </tr>
                  ) : filteredAssignmentSubjects.length === 0 ? (
                    <tr>
                      <td
                        colSpan={4}
                        className="subjects-empty"
                      >
                        No matching subjects.
                      </td>
                    </tr>
                  ) : (
                    filteredAssignmentSubjects.map(
                      (subject, index) => {
                        const offered =
                          offeredSubjectSet.has(subject.id);

                        return (
                          <tr key={subject.id}>
                            <td>{index + 1}</td>
                            <td>
                              <strong>{subject.name}</strong>
                            </td>
                            <td>
                              <span
                                className={`subjects-status ${
                                  offered
                                    ? "assigned"
                                    : "unassigned"
                                }`}
                              >
                                {offered
                                  ? "Assigned"
                                  : "Not Assigned"}
                              </span>
                            </td>
                            <td>
                              <button
                                type="button"
                                className={
                                  offered
                                    ? "subjects-remove-button"
                                    : "subjects-assign-button"
                                }
                                disabled={
                                  !canEditOfferings ||
                                  saving ||
                                  loadingOfferings
                                }
                                onClick={() =>
                                  void handleToggleOffering(
                                    subject,
                                    offered
                                  )
                                }
                              >
                                {offered
                                  ? "Remove"
                                  : "Assign Subject"}
                              </button>
                            </td>
                          </tr>
                        );
                      }
                    )
                  )}
                </tbody>
              </table>
            </div>
          </>
        )}
      </section>

      {(dialog === "create" || dialog === "edit") && (
        <div className="subjects-modal-overlay">
          <div
            className="subjects-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="subject-dialog-title"
          >
            <div className="subjects-modal-header">
              <div>
                <h2 id="subject-dialog-title">
                  {dialog === "create"
                    ? "Add New Subject"
                    : "Edit Subject"}
                </h2>
                <p>
                  {dialog === "create"
                    ? "Create a new subject in the school catalogue."
                    : "Update the selected subject details."}
                </p>
              </div>
              <button
                type="button"
                onClick={closeDialog}
                disabled={saving}
                aria-label="Close dialog"
              >
                <X size={20} />
              </button>
            </div>

            <form onSubmit={(event) =>
              void handleSaveSubject(event)
            }>
              <div className="subjects-modal-body">
                <label htmlFor="subject-name">
                  Subject Name <span>*</span>
                </label>
                <input
                  id="subject-name"
                  required
                  autoFocus
                  value={subjectName}
                  placeholder="e.g. Mathematics"
                  onChange={(event) =>
                    setSubjectName(event.target.value)
                  }
                />

                {error && (
                  <p className="subjects-error" role="alert">
                    {error}
                  </p>
                )}
              </div>

              <div className="subjects-modal-footer">
                <button
                  type="button"
                  className="subjects-cancel-button"
                  disabled={saving}
                  onClick={closeDialog}
                >
                  Cancel
                </button>

                <button
                  type="submit"
                  className="subjects-primary-button"
                  disabled={saving}
                >
                  {saving
                    ? "Saving..."
                    : dialog === "create"
                      ? "Create Subject"
                      : "Update Subject"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {dialog === "delete" && selectedSubject && (
        <div className="subjects-modal-overlay">
          <div
            className="subjects-modal"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="delete-subject-title"
          >
            <div className="subjects-modal-header">
              <div>
                <h2 id="delete-subject-title">
                  Delete Subject
                </h2>
                <p>
                  Confirm permanent subject deletion.
                </p>
              </div>
              <button
                type="button"
                aria-label="Close dialog"
                disabled={saving}
                onClick={closeDialog}
              >
                <X size={20} />
              </button>
            </div>

            <div className="subjects-modal-body">
              <p>
                Are you sure you want to delete{" "}
                <strong>{selectedSubject.name}</strong>?
              </p>

              <p className="subjects-delete-warning">
                This cannot be undone. Subjects referenced by
                class offerings, teacher assignments, or
                academic results cannot be deleted.
              </p>

              {error && (
                <p className="subjects-error" role="alert">
                  {error}
                </p>
              )}
            </div>

            <div className="subjects-modal-footer">
              <button
                type="button"
                className="subjects-cancel-button"
                disabled={saving}
                onClick={closeDialog}
              >
                Cancel
              </button>
              <button
                type="button"
                className="subjects-confirm-delete"
                disabled={saving}
                onClick={() => void handleDeleteSubject()}
              >
                <Trash2 size={16} />
                {saving ? "Deleting..." : "Delete Subject"}
              </button>
            </div>
          </div>
        </div>
      )}
    </main>
  );
}

export default Subjects;
