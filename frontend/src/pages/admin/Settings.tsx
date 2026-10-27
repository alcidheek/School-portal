
import { useEffect,
  useRef,
  useState,
  type FormEvent,
  type ChangeEvent, } from "react";
import { Link, useSearchParams } from "react-router-dom";

import {
  Building2,
  CalendarDays,
  CheckCircle2,
  ChevronRight,
  GraduationCap,
  KeyRound,
  Layers3,
  LockKeyhole,
  Mail,
  MapPin,
  Phone,
  School,
  Settings2,
  ShieldCheck,
  Users,
  BookOpen,
} from "lucide-react";

import {
  getAcademicSessions,
  getActiveTerm,
  getClasses,
  getSubjects,
  getUsers,
  type AcademicSession,
  type AcademicTerm,
  getSchoolProfile,
updateSchoolProfile,
type UpdateSchoolProfileRequest,
} from "../../services/adminService.ts";

import {
  getMediaObjectUrl,
  uploadMedia,
  deleteMedia,
  validateImage,
} from "../../services/mediaService.ts";

import AccountSecurity from "./AccountSecurity";
import "./Settings.css";

type SettingsTab =
  | "school-profile"
  | "account-security"
  | "system-overview";

const settingsTabs: {
  id: SettingsTab;
  label: string;
  icon: typeof Building2;
}[] = [
  {
    id: "school-profile",
    label: "School Profile",
    icon: Building2,
  },
  {
    id: "account-security",
    label: "Account & Security",
    icon: ShieldCheck,
  },
  {
    id: "system-overview",
    label: "System Overview",
    icon: Settings2,
  },
];

function formatTerm(name?: string) {
  return name
    ? name
        .replaceAll("_", " ")
        .toLowerCase()
        .replace(/\b\w/g, (letter) => letter.toUpperCase())
    : "Not configured";
}

/* =========================================
   SCHOOL PROFILE
========================================= */


const emptySchoolProfile: UpdateSchoolProfileRequest = {
  name: "",
  motto: "",
  email: "",
  phone: "",
  address: "",
  website: "",
};

const schoolFields: {
  key: keyof UpdateSchoolProfileRequest;
  label: string;
  type?: string;
  fullWidth?: boolean;
}[] = [
  { key: "name", label: "School Name" },
  { key: "motto", label: "School Motto" },
  { key: "email", label: "School Email", type: "email" },
  { key: "phone", label: "Phone Number", type: "tel" },
  { key: "address", label: "School Address", fullWidth: true },
  {
    key: "website",
    label: "School Website",
    type: "url",
    fullWidth: true,
  },
];

function SchoolProfile() {
  const [profile, setProfile] =
    useState<UpdateSchoolProfileRequest | null>(null);

  const [form, setForm] =
    useState<UpdateSchoolProfileRequest>(emptySchoolProfile);

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [editing, setEditing] = useState(false);

  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  const logoInputRef = useRef<HTMLInputElement>(null);

const [logoUrl, setLogoUrl] = useState<string | null>(null);
const [logoLoading, setLogoLoading] = useState(true);
const [logoBusy, setLogoBusy] = useState(false);
const [logoError, setLogoError] = useState("");
const [logoSuccess, setLogoSuccess] = useState("");
const logoUrlRef = useRef<string | null>(null);



  useEffect(() => {
    let active = true;

    async function loadProfile() {
      try {
        const result = await getSchoolProfile();

        if (!active) return;

        const data = result
          ? {
              name: result.name,
              motto: result.motto,
              email: result.email,
              phone: result.phone,
              address: result.address,
              website: result.website,
            }
          : null;

        setProfile(data);
        setForm(data ?? { ...emptySchoolProfile });
      } catch (err) {
        if (!active) return;

        setError(
          err instanceof Error
            ? err.message
            : "Failed to load school profile."
        );
      } finally {
        if (active) setLoading(false);
      }
    }

    void loadProfile();

    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
  const controller = new AbortController();
  let active = true;

  async function loadLogo() {
    try {
      const url = await getMediaObjectUrl(
        "school-logo",
        controller.signal
      );

      if (!active) {
        if (url) URL.revokeObjectURL(url);
        return;
      }

      logoUrlRef.current = url;
      setLogoUrl(url);
    } catch (err) {
      if (active && !controller.signal.aborted) {
        setLogoError(
          err instanceof Error
            ? err.message
            : "Failed to load school logo."
        );
      }
    } finally {
      if (active) setLogoLoading(false);
    }
  }

  void loadLogo();

  return () => {
    active = false;
    controller.abort();

    if (logoUrlRef.current) {
      URL.revokeObjectURL(logoUrlRef.current);
      logoUrlRef.current = null;
    }
  };
}, []);

function replaceLogoUrl(url: string | null) {
  const previous = logoUrlRef.current;
  logoUrlRef.current = url;
  setLogoUrl(url);

  if (previous && previous !== url) {
    URL.revokeObjectURL(previous);
  }
}

async function handleLogoUpload(
  event: ChangeEvent<HTMLInputElement>
) {
  const file = event.target.files?.[0];
  if (!file || logoBusy) return;

  const validationError = validateImage(file);

  if (validationError) {
    setLogoError(validationError);
    event.target.value = "";
    return;
  }

  setLogoBusy(true);
  setLogoError("");
  setLogoSuccess("");

  try {
    await uploadMedia("school-logo", file);

    const newUrl = await getMediaObjectUrl("school-logo");
    replaceLogoUrl(newUrl);

    setLogoSuccess("School logo updated successfully.");
  } catch (err) {
    setLogoError(
      err instanceof Error
        ? err.message
        : "Failed to upload school logo."
    );
  } finally {
    setLogoBusy(false);
    event.target.value = "";
  }
}

async function handleLogoDelete() {
  if (logoBusy) return;

  setLogoBusy(true);
  setLogoError("");
  setLogoSuccess("");

  try {
    await deleteMedia("school-logo");
    replaceLogoUrl(null);
    setLogoSuccess("School logo removed successfully.");
  } catch (err) {
    setLogoError(
      err instanceof Error
        ? err.message
        : "Failed to remove school logo."
    );
  } finally {
    setLogoBusy(false);
  }
}

  function handleChange(
    field: keyof UpdateSchoolProfileRequest,
    value: string
  ) {
    setForm((previous) => ({
      ...previous,
      [field]: value,
    }));

    setError("");
    setSuccess("");
  }

  function handleCancel() {
    setForm(profile ? { ...profile } : { ...emptySchoolProfile });
    setEditing(false);
    setError("");
    setSuccess("");
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (saving) return;

    const payload: UpdateSchoolProfileRequest = {
      name: form.name.trim(),
      motto: form.motto.trim(),
      email: form.email.trim(),
      phone: form.phone.trim(),
      address: form.address.trim(),
      website: form.website.trim(),
    };

    if (!payload.name) {
      setError("School name is required.");
      return;
    }

    setSaving(true);
    setError("");
    setSuccess("");

    try {
      const updated = await updateSchoolProfile(payload);

      const saved: UpdateSchoolProfileRequest = {
        name: updated.name,
        motto: updated.motto,
        email: updated.email,
        phone: updated.phone,
        address: updated.address,
        website: updated.website,
      };

      setProfile(saved);
      setForm(saved);
      setEditing(false);
      setSuccess("School profile updated successfully.");
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Unable to save school profile."
      );
    } finally {
      setSaving(false);
    }
  }

  const displayed = editing ? form : profile;

  const displayValue = (value?: string) =>
    value?.trim() || "Not configured";

  return (
    <div className="settings-content-grid">
      <section className="settings-panel">
        <div className="settings-panel-heading">
          <div className="settings-heading-icon">
            <School size={21} />
          </div>

          <div>
            <h2>School Information</h2>
            <p>
              Manage your school's basic information and identity.
            </p>
          </div>
        </div>

        {loading ? (
          <p>Loading school information...</p>
        ) : (
          <>

            <div className="settings-school-identity">
  <div className="settings-school-logo">
    {logoUrl ? (
      <img src={logoUrl} alt="School logo" />
    ) : (
      <GraduationCap size={36} />
    )}
  </div>

  <div>
    <h3>{displayValue(displayed?.name)}</h3>
    <p>{displayValue(displayed?.motto)}</p>
  </div>
</div>

<div className="settings-logo-management">
  <input
    ref={logoInputRef}
    type="file"
    accept="image/png,image/jpeg,image/webp"
    onChange={handleLogoUpload}
    style={{ display: "none" }}
    aria-label="Choose school logo"
  />

  <button
    type="button"
    disabled={logoBusy || logoLoading}
    onClick={() => logoInputRef.current?.click()}
  >
    {logoBusy
      ? "Processing..."
      : logoUrl
        ? "Change Logo"
        : "Upload Logo"}
  </button>

  {logoUrl && (
    <button
      type="button"
      disabled={logoBusy}
      onClick={handleLogoDelete}
    >
      Remove Logo
    </button>
  )}

  <p>PNG, JPEG or WebP. Maximum 2 MiB.</p>

  {logoError && (
    <p className="settings-error" role="alert">
      {logoError}
    </p>
  )}

  {logoSuccess && (
    <p className="settings-success" role="status">
      {logoSuccess}
    </p>
  )}
</div>

            {error && (
              <p className="settings-error" role="alert">
                {error}
              </p>
            )}

            {success && (
              <p role="status" className="settings-success">
                {success}
              </p>
            )}

            {!editing && (
              <button
                type="button"
                className="settings-action-link"
                onClick={() => {
                  setEditing(true);
                  setSuccess("");
                  setError("");
                }}
              >
                Edit School Profile
              </button>
            )}

            <form onSubmit={handleSave}>
              <div className="settings-field-grid">
                {schoolFields.map((field) => (
                  <div
                    key={field.key}
                    className={`settings-info-field ${
                      field.fullWidth
                        ? "settings-field-full"
                        : ""
                    }`}
                  >
                    <label htmlFor={`school-${field.key}`}>
                      {field.label}
                    </label>

                    {editing ? (
                      <input
                        id={`school-${field.key}`}
                        type={field.type ?? "text"}
                        value={form[field.key]}
                        onChange={(event) =>
                          handleChange(
                            field.key,
                            event.target.value
                          )
                        }
                        required={field.key === "name"}
                        disabled={saving}
                      />
                    ) : (
                      <strong>
                        {displayValue(profile?.[field.key])}
                      </strong>
                    )}
                  </div>
                ))}
              </div>

              {editing && (
                <div className="settings-profile-actions">
                  <button type="submit" disabled={saving}>
                    {saving ? "Saving..." : "Save Changes"}
                  </button>

                  <button
                    type="button"
                    onClick={handleCancel}
                    disabled={saving}
                  >
                    Cancel
                  </button>
                </div>
              )}
            </form>
          </>
        )}
      </section>

      <aside className="settings-side-column">
        <section className="settings-panel">
          <h2>School Preview</h2>

          <div className="settings-preview">
          <div className="settings-preview-logo">
  {logoUrl ? (
    <img src={logoUrl} alt="School logo preview" />
  ) : (
    <GraduationCap size={34} />
  )}
</div>

            <h3>{displayValue(displayed?.name)}</h3>
            <p>
              {displayValue(displayed?.motto)}
            </p>

            <div className="settings-preview-divider" />

            <span>School identity preview</span>
          </div>
        </section>

        <section className="settings-panel">
          <h2>School Contact Details</h2>

          <div className="settings-contact-row">
            <Mail size={18} />
            <span>{displayValue(displayed?.email)}</span>
          </div>

          <div className="settings-contact-row">
            <Phone size={18} />
            <span>{displayValue(displayed?.phone)}</span>
          </div>

          <div className="settings-contact-row">
            <MapPin size={18} />
            <span>{displayValue(displayed?.address)}</span>
          </div>
        </section>
      </aside>
    </div>
  );
}


/* =========================================
   ACCOUNT AND SECURITY
========================================= */

function AccountSecuritySettings() {
  return (
    <div className="settings-account-content">
      <div className="settings-section-intro">
        <div className="settings-heading-icon">
          <LockKeyhole size={23} />
        </div>

        <div>
          <h2>Account & Security</h2>
          <p>
            Manage your administrator login email and
            password securely.
          </p>
        </div>
      </div>

      <div className="settings-security-notice">
        <ShieldCheck size={21} />

        <div>
          <strong>Protect your administrator account</strong>
          <p>
            Changing your login email requires your current
            password. Changing your password will require
            you to sign in again.
          </p>
        </div>
      </div>

      <div className="settings-security-feature-list">
        <div>
          <Mail size={19} />
          <span>Secure login email updates</span>
        </div>

        <div>
          <KeyRound size={19} />
          <span>Password protection</span>
        </div>

        <div>
          <CheckCircle2 size={19} />
          <span>Current password verification</span>
        </div>
      </div>

      {/*
        Reuse the existing component and its backend
        integrations without changing security behavior.
      */}
      <AccountSecurity />
    </div>
  );
}

/* =========================================
   SYSTEM OVERVIEW
========================================= */

function SystemOverview() {
  const [sessions, setSessions] = useState<
    AcademicSession[]
  >([]);

  const [activeTerm, setActiveTerm] =
    useState<AcademicTerm | null>(null);

  const [classCount, setClassCount] = useState<number | null>(
    null
  );

  const [subjectCount, setSubjectCount] = useState<
    number | null
  >(null);

  const [studentCount, setStudentCount] = useState<
    number | null
  >(null);

  const [teacherCount, setTeacherCount] = useState<
    number | null
  >(null);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;

    async function loadSystemOverview() {
      setLoading(true);
      setError("");

      const results = await Promise.allSettled([
        getAcademicSessions(),
        getClasses(),
        getSubjects(),
        getUsers(),
      ]);

      if (!active) return;

      const [
        sessionResult,
        classResult,
        subjectResult,
        userResult,
      ] = results;

      let failedRequests = 0;

      if (sessionResult.status === "fulfilled") {
        setSessions(sessionResult.value);

        const currentSession = sessionResult.value.find(
          (session) => session.is_active
        );

        if (currentSession) {
          try {
            const term = await getActiveTerm(
              currentSession.id
            );

            if (active) setActiveTerm(term);
          } catch {
            failedRequests += 1;

            if (active) setActiveTerm(null);
          }
        } else {
          setActiveTerm(null);
        }
      } else {
        failedRequests += 1;
      }

      if (!active) return;

      if (classResult.status === "fulfilled") {
        setClassCount(classResult.value.length);
      } else {
        failedRequests += 1;
      }

      if (subjectResult.status === "fulfilled") {
        setSubjectCount(subjectResult.value.length);
      } else {
        failedRequests += 1;
      }

      if (userResult.status === "fulfilled") {
        setStudentCount(
          userResult.value.filter(
            (user) => user.role === "STUDENT"
          ).length
        );

        setTeacherCount(
          userResult.value.filter(
            (user) => user.role === "TEACHER"
          ).length
        );
      } else {
        failedRequests += 1;
      }

      if (failedRequests > 0) {
        setError(
          "Some system information could not be loaded. Please refresh to try again."
        );
      }

      setLoading(false);
    }

    void loadSystemOverview();

    return () => {
      active = false;
    };
  }, []);

  const currentSession = sessions.find(
    (session) => session.is_active
  );

  return (
    <div className="settings-overview">
      <section className="settings-overview-cards">
        <div className="settings-overview-stat">
          <div className="settings-stat-icon blue">
            <Users size={24} />
          </div>

          <span>Registered Students</span>
          <strong>
            {loading ? "…" : studentCount ?? "—"}
          </strong>
        </div>

        <div className="settings-overview-stat">
          <div className="settings-stat-icon green">
            <GraduationCap size={24} />
          </div>

          <span>Registered Teachers</span>
          <strong>
            {loading ? "…" : teacherCount ?? "—"}
          </strong>
        </div>

        <div className="settings-overview-stat">
          <div className="settings-stat-icon orange">
            <Layers3 size={24} />
          </div>

          <span>Total Classes</span>
          <strong>
            {loading ? "…" : classCount ?? "—"}
          </strong>
        </div>

        <div className="settings-overview-stat">
          <div className="settings-stat-icon purple">
            <BookOpen size={24} />
          </div>

          <span>Total Subjects</span>
          <strong>
            {loading ? "…" : subjectCount ?? "—"}
          </strong>
        </div>
      </section>

      {error && (
        <p className="settings-error" role="alert">
          {error}
        </p>
      )}

      <div className="settings-content-grid">
        <section className="settings-panel">
          <div className="settings-panel-heading">
            <div className="settings-heading-icon">
              <CalendarDays size={21} />
            </div>

            <div>
              <h2>Current Academic Period</h2>
              <p>
                Overview of the active academic session
                and term.
              </p>
            </div>
          </div>

          <div className="settings-academic-details">
            <div>
              <span>Academic Session</span>
              <strong>
                {loading
                  ? "Loading…"
                  : currentSession?.name ??
                    "No active session"}
              </strong>
            </div>

            <div>
              <span>Current Term</span>
              <strong>
                {loading
                  ? "Loading…"
                  : formatTerm(activeTerm?.name)}
              </strong>
            </div>

            <div>
              <span>Session Status</span>
              <strong>
                {loading
                  ? "Loading…"
                  : currentSession
                    ? "Active"
                    : "Inactive"}
              </strong>
            </div>
          </div>

          <Link
            className="settings-action-link"
            to="/admin/academic-sessions"
          >
            Manage Academic Sessions
            <ChevronRight size={18} />
          </Link>
        </section>

        <section className="settings-panel">
          <h2>Quick Navigation</h2>
          <p className="settings-panel-description">
            Access frequently used administration pages.
          </p>

          <div className="settings-quick-links">
            <Link to="/admin/users">
              <Users size={19} />
              <span>Manage Users</span>
              <ChevronRight size={17} />
            </Link>

            <Link to="/admin/classes">
              <Layers3 size={19} />
              <span>Manage Classes</span>
              <ChevronRight size={17} />
            </Link>

            <Link to="/admin/subjects">
              <BookOpen size={19} />
              <span>Manage Subjects</span>
              <ChevronRight size={17} />
            </Link>

            <Link to="/admin/teacher-assignments">
              <GraduationCap size={19} />
              <span>Teacher Assignments</span>
              <ChevronRight size={17} />
            </Link>

            <Link to="/admin/results">
              <CheckCircle2 size={19} />
              <span>Results Management</span>
              <ChevronRight size={17} />
            </Link>
          </div>
        </section>
      </div>
    </div>
  );
}

/* =========================================
   MAIN SETTINGS PAGE
========================================= */

export default function Settings() {
  const [searchParams, setSearchParams] =
    useSearchParams();

  const requestedTab = searchParams.get("tab");

  const activeTab: SettingsTab =
    requestedTab === "account-security" ||
    requestedTab === "system-overview" ||
    requestedTab === "school-profile"
      ? requestedTab
      : "school-profile";

  function changeTab(tab: SettingsTab) {
    setSearchParams({ tab });
  }

  return (
    <main className="settings-page">
      <section className="settings-banner">
        <div>
          <h1>School Settings</h1>
          <p>
            Manage school information, administrator
            security, and system overview.
          </p>
        </div>

        <Settings2
          size={86}
          className="settings-banner-icon"
          aria-hidden="true"
        />
      </section>

      <nav
        className="settings-tabs"
        aria-label="Settings sections"
      >
        {settingsTabs.map((tab) => {
          const TabIcon = tab.icon;

          return (
            <button
              key={tab.id}
              type="button"
              className={
                activeTab === tab.id
                  ? "settings-tab active"
                  : "settings-tab"
              }
              aria-current={
                activeTab === tab.id
                  ? "page"
                  : undefined
              }
              onClick={() => changeTab(tab.id)}
            >
              <TabIcon size={19} />
              {tab.label}
            </button>
          );
        })}
      </nav>

      {activeTab === "school-profile" && (
        <SchoolProfile />
      )}

      {activeTab === "account-security" && (
        <AccountSecuritySettings />
      )}

      {activeTab === "system-overview" && (
        <SystemOverview />
      )}
    </main>
  );
}
