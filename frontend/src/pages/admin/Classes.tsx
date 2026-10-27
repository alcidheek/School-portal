
import { useEffect, useState } from "react";
import {
  BookOpen,
  GraduationCap,
  Layers,
  Pencil,
  Plus,
  Search,
  Trash2,
  X,
} from "lucide-react";

import {
  getClasses,
  createClass,
  updateClass,
  deleteClass,
  type SchoolClass,
  type ClassRequest,
} from "../../services/classService";

import "./Classes.css";

type DialogType = "create" | "edit" | "delete" | null;

const initialClassForm: ClassRequest = {
  name: "",
  level: "",
};

function formatDate(date: string): string {
  const parsedDate = new Date(date);

  if (Number.isNaN(parsedDate.getTime())) {
    return "—";
  }

  return parsedDate.toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
    timeZone: "UTC",
  });
}

function getErrorMessage(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "Something went wrong. Please try again.";
}

function Classes() {
  const [classes, setClasses] = useState<SchoolClass[]>([]);
  const [search, setSearch] = useState("");

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const [dialog, setDialog] = useState<DialogType>(null);
  const [selectedClass, setSelectedClass] =
    useState<SchoolClass | null>(null);

  const [classForm, setClassForm] =
    useState<ClassRequest>(initialClassForm);

  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const juniorClasses = classes.filter((schoolClass) =>
    schoolClass.level.trim().toUpperCase().startsWith("JSS")
  );

  const seniorClasses = classes.filter((schoolClass) => {
    const level = schoolClass.level.trim().toUpperCase();

    return level.startsWith("SS") || level.startsWith("SSS");
  });

  const lastCreatedClass =
    [...classes].sort(
      (firstClass, secondClass) =>
        new Date(secondClass.created_at).getTime() -
        new Date(firstClass.created_at).getTime()
    )[0] ?? null;

  const filteredClasses = classes.filter((schoolClass) => {
    const searchValue = search.toLowerCase().trim();

    return (
      schoolClass.name.toLowerCase().includes(searchValue) ||
      schoolClass.level.toLowerCase().includes(searchValue)
    );
  });

  useEffect(() => {
    let active = true;

    async function loadClasses() {
      try {
        const classRecords = await getClasses();

        if (active) {
          setClasses(classRecords);
        }
      } catch (error) {
        if (active) {
          setError(getErrorMessage(error));
        }
      } finally {
        if (active) {
          setLoading(false);
        }
      }
    }

    void loadClasses();

    return () => {
      active = false;
    };
  }, []);

  async function refreshClasses() {
    const classRecords = await getClasses();
    setClasses(classRecords);
  }

  function openCreateDialog() {
    setSelectedClass(null);
    setClassForm({ ...initialClassForm });
    setError("");
    setDialog("create");
  }

  function openEditDialog(schoolClass: SchoolClass) {
    setSelectedClass(schoolClass);

    setClassForm({
      name: schoolClass.name,
      level: schoolClass.level,
    });

    setError("");
    setDialog("edit");
  }

  function openDeleteDialog(schoolClass: SchoolClass) {
    setSelectedClass(schoolClass);
    setError("");
    setDialog("delete");
  }

  function closeDialog() {
    if (saving) return;

    setDialog(null);
    setSelectedClass(null);
    setError("");
  }

  async function handleSaveClass(
    event: React.FormEvent<HTMLFormElement>
  ) {
    event.preventDefault();

    const className = classForm.name.trim();
    const classLevel = classForm.level.trim();

    if (!className || !classLevel) {
      setError("Class name and level are required.");
      return;
    }

    setSaving(true);
    setError("");
    setMessage("");

    try {
      if (dialog === "edit" && selectedClass) {
        await updateClass(selectedClass.id, {
          name: className,
          level: classLevel,
        });

        setMessage("Class updated successfully.");
      } else if (dialog === "create") {
        await createClass({
          name: className,
          level: classLevel,
        });

        setMessage("Class created successfully.");
      } else {
        return;
      }

      await refreshClasses();
      closeDialog();
    } catch (error) {
      setError(getErrorMessage(error));
    } finally {
      setSaving(false);
    }
  }

  async function handleDeleteClass() {
    if (!selectedClass || saving) return;

    setSaving(true);
    setError("");
    setMessage("");

    try {
      await deleteClass(selectedClass.id);
      await refreshClasses();

      setMessage(
        `${selectedClass.name} deleted successfully.`
      );

      setDialog(null);
      setSelectedClass(null);
    } catch (error) {
      setError(getErrorMessage(error));
    } finally {
      setSaving(false);
    }
  }

  return (
    <main className="classes-page">
      {/* Page banner */}
      <section className="classes-banner">
        <div className="classes-banner-content">
          <h1>Classes Management</h1>
          <p>
            Manage school classes used for enrollment,
            subject offerings, and teacher assignment.
          </p>
        </div>

        <GraduationCap
          size={100}
          className="classes-banner-icon"
          aria-hidden="true"
        />
      </section>

      {/* Overview cards */}
      <section className="classes-stats">
        <div className="classes-stat-card">
          <div className="classes-stat-icon blue">
            <Layers size={27} />
          </div>
          <div>
            <span>Total Classes</span>
            <strong>{loading ? "—" : classes.length}</strong>
            <small>All school classes</small>
          </div>
        </div>

        <div className="classes-stat-card">
          <div className="classes-stat-icon green">
            <GraduationCap size={27} />
          </div>
          <div>
            <span>JSS Classes</span>
            <strong>{loading ? "—" : juniorClasses.length}</strong>
            <small>Junior Secondary School</small>
          </div>
        </div>

        <div className="classes-stat-card">
          <div className="classes-stat-icon purple">
            <GraduationCap size={27} />
          </div>
          <div>
            <span>SS Classes</span>
            <strong>{loading ? "—" : seniorClasses.length}</strong>
            <small>Senior Secondary School</small>
          </div>
        </div>

        <div className="classes-stat-card">
          <div className="classes-stat-icon orange">
            <BookOpen size={27} />
          </div>
          <div>
            <span>Last Created</span>
            <strong>
              {loading ? "—" : lastCreatedClass?.name ?? "—"}
            </strong>
            <small>
              {lastCreatedClass
                ? formatDate(lastCreatedClass.created_at)
                : "No classes created"}
            </small>
          </div>
        </div>
      </section>

      {/* Classes table */}
      <section className="classes-panel">
        <div className="classes-panel-heading">
          <div>
            <h2>School Classes</h2>
            <p>View, create and manage school classes.</p>
          </div>

          <button
            type="button"
            className="classes-primary-button"
            onClick={openCreateDialog}
            disabled={saving}
          >
            <Plus size={18} />
            Create Class
          </button>
        </div>

        <div className="classes-toolbar">
          <div className="classes-search">
            <Search size={18} />
            <input
              type="search"
              placeholder="Search classes by name or level..."
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              aria-label="Search classes"
            />
          </div>
        </div>

        {message && (
          <p className="classes-success" role="status">
            {message}
          </p>
        )}

        {error && !dialog && (
          <p className="classes-error" role="alert">
            {error}
          </p>
        )}

        <div className="classes-table-wrapper">
          <table className="classes-table">
            <thead>
              <tr>
                <th>#</th>
                <th>Class Name</th>
                <th>Class Level</th>
                <th>Created At</th>
                <th>Updated At</th>
                <th>Actions</th>
              </tr>
            </thead>

            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={6} className="classes-empty">
                    Loading school classes...
                  </td>
                </tr>
              ) : filteredClasses.length === 0 ? (
                <tr>
                  <td colSpan={6} className="classes-empty">
                    {search
                      ? "No classes match your search."
                      : "No school classes have been created yet."}
                  </td>
                </tr>
              ) : (
                filteredClasses.map((schoolClass, index) => (
                  <tr key={schoolClass.id}>
                    <td>{index + 1}</td>
                    <td>
                      <strong>{schoolClass.name}</strong>
                    </td>
                    <td>{schoolClass.level}</td>
                    <td>{formatDate(schoolClass.created_at)}</td>
                    <td>{formatDate(schoolClass.updated_at)}</td>
                    <td>
                      <div className="classes-table-actions">
                        <button
                          type="button"
                          className="classes-edit-button"
                          onClick={() => openEditDialog(schoolClass)}
                          disabled={saving}
                        >
                          <Pencil size={15} />
                          Edit
                        </button>

                        <button
                          type="button"
                          className="classes-delete-button"
                          onClick={() => openDeleteDialog(schoolClass)}
                          disabled={saving}
                        >
                          <Trash2 size={15} />
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>

      {/* Create / Edit dialog */}
      {(dialog === "create" || dialog === "edit") && (
        <div className="classes-modal-overlay">
          <div
            className="classes-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="classes-dialog-title"
          >
            <div className="classes-modal-heading">
              <div>
                <h2 id="classes-dialog-title">
                  {dialog === "create"
                    ? "Create New Class"
                    : "Edit Class"}
                </h2>
                <p>
                  {dialog === "create"
                    ? "Add a new school class to the system."
                    : "Update the class details."}
                </p>
              </div>

              <button
                type="button"
                className="classes-close-button"
                onClick={closeDialog}
                disabled={saving}
                aria-label="Close dialog"
              >
                <X size={20} />
              </button>
            </div>

            <form onSubmit={(event) => void handleSaveClass(event)}>
              <div className="classes-modal-body">
                <label htmlFor="class-name">
                  Class Name <span>*</span>
                </label>
                <input
                  id="class-name"
                  type="text"
                  required
                  placeholder="e.g. JSS 1A"
                  value={classForm.name}
                  onChange={(event) =>
                    setClassForm({
                      ...classForm,
                      name: event.target.value,
                    })
                  }
                />

                <label htmlFor="class-level">
                  Class Level <span>*</span>
                </label>
                <select
                  id="class-level"
                  required
                  value={classForm.level}
                  onChange={(event) =>
                    setClassForm({
                      ...classForm,
                      level: event.target.value,
                    })
                  }
                >
                  <option value="">Select class level</option>
                  {[
                    "JSS 1",
                    "JSS 2",
                    "JSS 3",
                    "SS 1",
                    "SS 2",
                    "SS 3",
                  ].map((level) => (
                    <option key={level} value={level}>
                      {level}
                    </option>
                  ))}
                  {classForm.level &&
                    ![
                      "JSS 1",
                      "JSS 2",
                      "JSS 3",
                      "SS 1",
                      "SS 2",
                      "SS 3",
                    ].includes(classForm.level) && (
                      <option value={classForm.level}>
                        {classForm.level}
                      </option>
                    )}
                </select>

                {error && (
                  <p className="classes-error" role="alert">
                    {error}
                  </p>
                )}
              </div>

              <div className="classes-modal-actions">
                <button
                  type="button"
                  className="classes-cancel-button"
                  onClick={closeDialog}
                  disabled={saving}
                >
                  Cancel
                </button>

                <button
                  type="submit"
                  className="classes-primary-button"
                  disabled={saving}
                >
                  {saving
                    ? "Saving..."
                    : dialog === "create"
                      ? "Create Class"
                      : "Update Class"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete confirmation */}
      {dialog === "delete" && selectedClass && (
        <div className="classes-modal-overlay">
          <div
            className="classes-modal"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="delete-class-title"
          >
            <div className="classes-modal-heading">
              <div>
                <h2 id="delete-class-title">Delete Class</h2>
                <p>Confirm class deletion</p>
              </div>

              <button
                type="button"
                className="classes-close-button"
                onClick={closeDialog}
                disabled={saving}
                aria-label="Close dialog"
              >
                <X size={20} />
              </button>
            </div>

            <div className="classes-modal-body">
              <p className="classes-delete-warning">
                Are you sure you want to delete{" "}
                <strong>{selectedClass.name}</strong>?
              </p>

              <p className="classes-delete-description">
                This action cannot be undone. A class can only
                be deleted if it is not referenced by student
                enrollments, results, attendance, subject
                offerings, or teacher assignments.
              </p>

              {error && (
                <p className="classes-error" role="alert">
                  {error}
                </p>
              )}
            </div>

            <div className="classes-modal-actions">
              <button
                type="button"
                className="classes-cancel-button"
                onClick={closeDialog}
                disabled={saving}
              >
                Cancel
              </button>

              <button
                type="button"
                className="classes-confirm-delete"
                onClick={() => void handleDeleteClass()}
                disabled={saving}
              >
                <Trash2 size={16} />
                {saving ? "Deleting..." : "Delete Class"}
              </button>
            </div>
          </div>
        </div>
      )}
    </main>
  );
}

export default Classes;
