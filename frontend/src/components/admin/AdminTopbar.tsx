import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Bell, ChevronDown, Menu, Search, X } from "lucide-react";
import { currentAccount } from "../../services/accountSecurityService";
import { getClasses, getUsers, type AdminUser, type SchoolClass } from "../../services/adminService";
import "./AdminTopbar.css";
import { getMediaObjectUrl } from "../../services/mediaService.ts";

type AdminTopbarProps = { collapsed?: boolean; onToggle?: () => void };

export default function AdminTopbar({ collapsed = false, onToggle }: AdminTopbarProps) {
  const [identity, setIdentity] = useState<{ id: string; email: string } | null>(null);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [classes, setClasses] = useState<SchoolClass[]>([]);
  const [query, setQuery] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [profilePictureUrl, setProfilePictureUrl] = useState<string | null>(null);
const profilePictureRef = useRef<string | null>(null);

  useEffect(() => {
    let active = true;
    async function load() {
      try {
        const account = await currentAccount();
        if (!active) return;
        setIdentity(account);
        const [people, schoolClasses] = await Promise.allSettled([getUsers(), getClasses()]);
        if (!active) return;
        if (people.status === "fulfilled") setUsers(people.value);
        if (schoolClasses.status === "fulfilled") setClasses(schoolClasses.value);
        if (people.status === "rejected" || schoolClasses.status === "rejected") {
          setError("Some search records could not be loaded. Reload the page to try again.");
        }
      } catch {
        if (active) setError("Account information could not be loaded. Please sign in again.");
      } finally {
        if (active) setLoading(false);
      }
    }
    void load();
    return () => { active = false; };
  }, []);


useEffect(() => {
  let active = true;
  let requestVersion = 0;
  const controllers = new Set<AbortController>();

  async function loadProfilePicture() {
    const version = ++requestVersion;
    const controller = new AbortController();
    controllers.add(controller);

    try {
      const url = await getMediaObjectUrl(
        "profile-picture",
        controller.signal
      );

      if (!active || version !== requestVersion) {
        if (url) URL.revokeObjectURL(url);
        return;
      }

      const previousUrl = profilePictureRef.current;

      profilePictureRef.current = url;
      setProfilePictureUrl(url);

      if (previousUrl && previousUrl !== url) {
        URL.revokeObjectURL(previousUrl);
      }
    } catch (error) {
      if (active && !controller.signal.aborted) {
        console.error("Unable to load profile picture:", error);
      }
    } finally {
      controllers.delete(controller);
    }
  }

  function handlePictureUpdated() {
    void loadProfilePicture();
  }

  void loadProfilePicture();

  window.addEventListener(
    "blueprint:profile-picture-updated",
    handlePictureUpdated
  );

  return () => {
    active = false;
    requestVersion++;

    window.removeEventListener(
      "blueprint:profile-picture-updated",
      handlePictureUpdated
    );

    controllers.forEach((controller) => controller.abort());

    if (profilePictureRef.current) {
      URL.revokeObjectURL(profilePictureRef.current);
      profilePictureRef.current = null;
    }
  };
}, []);


  const profile = users.find(user => user.id === identity?.id);
  const name = profile?.full_name || identity?.email || (loading ? "Loading account…" : "Account unavailable");
  const initials = profile?.full_name.trim().split(/\s+/).slice(0, 2).map(part => part[0]).join("").toUpperCase()
    || identity?.email.slice(0, 1).toUpperCase() || "?";
  const term = query.trim().toLowerCase();
  const matchingUsers = term ? users.filter(user => user.role !== "ADMIN" &&
    [user.full_name, user.email, user.admission_number, user.employee_number].some(value => value?.toLowerCase().includes(term))) : [];
  const matchingClasses = term ? classes.filter(schoolClass => schoolClass.name.toLowerCase().includes(term)) : [];
  const resultsVisible = searchOpen && Boolean(term);

  return <header className="admin-topbar">
    <button type="button" className="topbar-toggle" onClick={onToggle} disabled={!onToggle}
      aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"} aria-expanded={!collapsed}>
      <Menu size={28} aria-hidden="true" />
    </button>
    <div className="topbar-search" onBlur={event => {
      if (!event.currentTarget.contains(event.relatedTarget)) setSearchOpen(false);
    }}>
      <form role="search" onSubmit={event => { event.preventDefault(); setSearchOpen(true); }}>
        <Search size={22} aria-hidden="true" />
        <input type="search" aria-label="Search students, teachers and classes" placeholder="Search students, teachers, classes…"
          value={query} onFocus={() => setSearchOpen(true)} onChange={event => { setQuery(event.target.value); setSearchOpen(true); }}
          onKeyDown={event => { if (event.key === "Escape") setSearchOpen(false); }} aria-controls={resultsVisible ? "admin-search-results" : undefined} />
      </form>
      {resultsVisible && <section id="admin-search-results" className="topbar-search-results" aria-label="Search results">
        <button type="button" className="topbar-search-close" onClick={() => setSearchOpen(false)} aria-label="Close search results"><X size={18} aria-hidden="true" /></button>
        <p role="status">{loading ? "Loading search records…" : `${matchingUsers.length + matchingClasses.length} matching records`}</p>
        {error && <p role="alert">{error}</p>}
        {!loading && !error && !matchingUsers.length && !matchingClasses.length && <p>No matching students, teachers or classes.</p>}
        <ul>{matchingUsers.slice(0, 5).map(user => <li key={user.id}><strong>{user.full_name}</strong><span>{user.role === "STUDENT" ? "Student" : "Teacher"} · {user.admission_number || user.employee_number || user.email}</span></li>)}
          {matchingClasses.slice(0, 5).map(schoolClass => <li key={schoolClass.id}><strong>{schoolClass.name}</strong><span>Class</span></li>)}</ul>
        <p>Showing up to five matches per category.</p>
        <div className="topbar-search-links"><Link to="/admin/users" onClick={() => setSearchOpen(false)}>Manage users</Link><Link to="/admin/class-subjects" onClick={() => setSearchOpen(false)}>Manage term configuration</Link></div>
      </section>}
    </div>
    <span className="topbar-notifications" role="img" aria-label="Notifications unavailable: no notification service is connected"
      title="Notifications unavailable: no notification service is connected"><Bell size={27} aria-hidden="true" /></span>
    <Link to="/admin/settings/account-security" className="admin-profile" aria-label={`Account settings for ${name}`}>

        <div className="admin-avatar" aria-hidden="true">
  {profilePictureUrl ? (
    <img
      src={profilePictureUrl}
      alt=""
      className="admin-avatar-image"
    />
  ) : (
    initials
  )}
</div>

      <div className="admin-profile-details"><strong>{name}</strong><span>{identity ? "Administrator" : "Account status unavailable"}</span></div>
      <ChevronDown size={18} aria-hidden="true" />
    </Link>
  </header>;
}
