
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  GraduationCap,
  ShieldCheck,
  UserRoundPlus,
  UsersRound,
  Search,
  Plus,
  MoreHorizontal,
} from "lucide-react";

import { getUsers, type AdminUser } from "../../services/adminService";
import "./UsersPage.css";

const PAGE_SIZE = 5;

function UsersPage() {
  const navigate = useNavigate();

  const [users, setUsers] = useState<AdminUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [roleFilter, setRoleFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [currentPage, setCurrentPage] = useState(1);
  const [selectedUsers, setSelectedUsers] = useState<string[]>([]);

  useEffect(() => {
    let active = true;

    getUsers()
      .then((data) => {
        if (active) setUsers(data);
      })
      .catch((err) => {
        if (active) {
          setError(
            err instanceof Error ? err.message : "Failed to load users"
          );
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, []);

  const counts = {
    total: users.length,
    students: users.filter((u) => u.role === "STUDENT").length,
    teachers: users.filter((u) => u.role === "TEACHER").length,
    admins: users.filter((u) => u.role === "ADMIN").length,
  };

  const stats = [
    { label: "Total Users", value: counts.total, icon: UsersRound, color: "blue" },
    { label: "Students", value: counts.students, icon: GraduationCap, color: "green" },
    { label: "Teachers", value: counts.teachers, icon: UserRoundPlus, color: "purple" },
    { label: "Administrators", value: counts.admins, icon: ShieldCheck, color: "orange" },
  ];

  const filteredUsers = users.filter((user) => {
    const term = search.trim().toLowerCase();

    const matchesSearch = [
      user.full_name,
      user.email,
      user.admission_number,
      user.employee_number,
      user.phone,
    ].some((value) => value?.toLowerCase().includes(term));

    const matchesRole = !roleFilter || user.role === roleFilter;

    const matchesStatus =
      !statusFilter ||
      (statusFilter === "ACTIVE" && user.is_active) ||
      (statusFilter === "INACTIVE" && !user.is_active);

    return matchesSearch && matchesRole && matchesStatus;
  });

  const totalPages = Math.ceil(filteredUsers.length / PAGE_SIZE);
  const startIndex = (currentPage - 1) * PAGE_SIZE;
  const paginatedUsers = filteredUsers.slice(
    startIndex,
    startIndex + PAGE_SIZE
  );

  const pageIds = paginatedUsers.map((user) => user.id);
  const allPageSelected =
    pageIds.length > 0 &&
    pageIds.every((id) => selectedUsers.includes(id));

  function toggleUser(id: string) {
    setSelectedUsers((current) =>
      current.includes(id)
        ? current.filter((userId) => userId !== id)
        : [...current, id]
    );
  }

  function togglePageSelection(checked: boolean) {
    setSelectedUsers((current) =>
      checked
        ? [...new Set([...current, ...pageIds])]
        : current.filter((id) => !pageIds.includes(id))
    );
  }

  function clearFilters() {
    setSearch("");
    setRoleFilter("");
    setStatusFilter("");
    setCurrentPage(1);
    setSelectedUsers([]);
  }

  function getInitials(name: string) {
    return name.trim().split(/\s+/).slice(0, 2)
      .map((part) => part[0]).join("").toUpperCase();
  }

  function formatDate(date: string) {
    const parsed = new Date(date);
    return Number.isNaN(parsed.getTime())
      ? "—"
      : parsed.toLocaleDateString("en-GB", {
          day: "2-digit",
          month: "short",
          year: "numeric",
        });
  }

  return (
    <main className="users-content">
      <header className="users-heading">
        <div className="users-heading-content">
          <h1>Users</h1>
          <p>
            Manage all students, teachers and staff accounts
            in your school.
          </p>
        </div>

        <UsersRound
          className="users-heading-icon"
          size={130}
          strokeWidth={1.2}
          aria-hidden="true"
        />
      </header>

      <section className="user-stats" aria-label="User statistics">
        {stats.map(({ label, value, icon: Icon, color }) => (
          <div className="user-stat-card" key={label}>
            <div className={`user-stat-icon ${color}`}>
              <Icon size={25} aria-hidden="true" />
            </div>
            <div>
              <p>{label}</p>
              <h3>{loading || error ? "—" : value}</h3>
              <small>Registered accounts</small>
            </div>
          </div>
        ))}
      </section>

      <section className="users-management-panel">
        <div className="users-panel-heading">
          <div>
            <h2>All Users</h2>
            <p>View and manage registered accounts</p>
          </div>
        </div>

        <div className="users-toolbar">
          <label className="users-search-box">
            <Search size={19} aria-hidden="true" />
            <input
              type="search"
              className="users-search"
              placeholder="Search by name, email or ID..."
              aria-label="Search users"
              value={search}
              onChange={(event) => {
                setSearch(event.target.value);
                setCurrentPage(1);
              }}
            />
          </label>

          <label className="users-filter-group">
            <span>Role</span>
            <select
              className="users-filter"
              value={roleFilter}
              onChange={(event) => {
                setRoleFilter(event.target.value);
                setCurrentPage(1);
              }}
            >
              <option value="">All Roles</option>
              <option value="STUDENT">Students</option>
              <option value="TEACHER">Teachers</option>
              <option value="ADMIN">Administrators</option>
            </select>
          </label>

          <label className="users-filter-group">
            <span>Status</span>
            <select
              className="users-filter"
              value={statusFilter}
              onChange={(event) => {
                setStatusFilter(event.target.value);
                setCurrentPage(1);
              }}
            >
              <option value="">All Status</option>
              <option value="ACTIVE">Active</option>
              <option value="INACTIVE">Inactive</option>
            </select>
          </label>

          <label className="users-filter-group">
            <span>Class</span>
            <select
              className="users-filter"
              disabled
              title="Class filtering will be available after enrollment integration"
            >
              <option>All Classes</option>
            </select>
          </label>

          <button
            type="button"
            className="clear-filters-button"
            onClick={clearFilters}
          >
            Clear Filters
          </button>

          <button
            type="button"
            className="add-user-button"
            onClick={() => navigate("/admin/users/new")}
          >
            <Plus size={18} aria-hidden="true" />
            Add User
          </button>
        </div>

        <div className="users-table-container">
          <table className="users-table">
            <thead>
              <tr>
                <th scope="col">
                  <input
                    type="checkbox"
                    aria-label="Select all users on this page"
                    checked={allPageSelected}
                    disabled={!pageIds.length}
                    onChange={(event) =>
                      togglePageSelection(event.target.checked)
                    }
                  />
                </th>
                <th scope="col">Name</th>
                <th scope="col">ID Number</th>
                <th scope="col">Role</th>
                <th scope="col">Class</th>
                <th scope="col">Status</th>
                <th scope="col">Date Created</th>
                <th scope="col">Actions</th>
              </tr>
            </thead>

            <tbody>
              {loading || error || filteredUsers.length === 0 ? (
                <tr>
                  <td colSpan={8} className="empty-users" role="status">
                    {loading
                      ? "Loading users..."
                      : error || "No users found."}
                  </td>
                </tr>
              ) : (
                paginatedUsers.map((user) => (
                  <tr key={user.id}>
                    <td>
                      <input
                        type="checkbox"
                        aria-label={`Select ${user.full_name}`}
                        checked={selectedUsers.includes(user.id)}
                        onChange={() => toggleUser(user.id)}
                      />
                    </td>

                    <td>
                      <div className="user-identity">
                        <div className="user-avatar">
                          {getInitials(user.full_name)}
                        </div>

                        <div className="user-identity-details">
                          <strong>{user.full_name}</strong>
                          <span>{user.email}</span>
                        </div>
                      </div>
                    </td>

                    <td>
                      {user.admission_number ||
                        user.employee_number ||
                        "—"}
                    </td>

                    <td>
                      <span
                        className={`user-role role-${user.role.toLowerCase()}`}
                      >
                        {user.role === "ADMIN"
                          ? "Admin"
                          : user.role === "STUDENT"
                            ? "Student"
                            : "Teacher"}
                      </span>
                    </td>

                    <td>—</td>

                    <td>
                      <span
                        className={`user-status ${
                          user.is_active ? "active" : "inactive"
                        }`}
                      >
                        <span className="status-dot" />
                        {user.is_active ? "Active" : "Inactive"}
                      </span>
                    </td>

                    <td>
                      {user.created_at
                        ? formatDate(user.created_at)
                        : "—"}
                    </td>

                    <td>
                      <button
                        type="button"
                        className="user-action-button"
                        title="User actions coming soon"
                        aria-label={`Actions for ${user.full_name} are not available yet`}
                        disabled
                      >
                        <MoreHorizontal size={20} aria-hidden="true" />
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {!loading && !error && filteredUsers.length > 0 && (
          <div className="users-pagination">
            <p>
              Showing {startIndex + 1}–
              {Math.min(
                startIndex + PAGE_SIZE,
                filteredUsers.length
              )}{" "}
              of {filteredUsers.length} users
            </p>

            <div className="pagination-buttons">
              <button
                type="button"
                disabled={currentPage === 1}
                onClick={() =>
                  setCurrentPage((page) => page - 1)
                }
              >
                Previous
              </button>

              {Array.from({ length: totalPages }, (_, index) => (
                <button
                  type="button"
                  key={index + 1}
                  className={
                    currentPage === index + 1 ? "active-page" : ""
                  }
                  aria-current={
                    currentPage === index + 1 ? "page" : undefined
                  }
                  onClick={() => setCurrentPage(index + 1)}
                >
                  {index + 1}
                </button>
              ))}

              <button
                type="button"
                disabled={currentPage === totalPages}
                onClick={() =>
                  setCurrentPage((page) => page + 1)
                }
              >
                Next
              </button>
            </div>
          </div>
        )}
      </section>
    </main>
  );
}

export default UsersPage;
