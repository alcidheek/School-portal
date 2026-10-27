import { useEffect, useState } from "react";
import { getMediaObjectUrl } from "../../services/mediaService.ts";
import { NavLink, useLocation } from "react-router-dom";
import type { LucideIcon } from "lucide-react";


import {
  BookOpen,
  CalendarDays,
  CalendarCheck,
  ChartNoAxesColumn,
  GraduationCap,
  House,
  Layers,
  LogOut,
  Settings,
  UserRoundPlus,
  UsersRound,
} from "lucide-react";

import LogoutButton from "../LogoutButton";
import "./AdminSidebar.css";



type AdminSidebarProps = {
  collapsed: boolean;
  onToggle: () => void;
};

type SidebarItem = {
  label: string;
  icon: LucideIcon;
  path?: string;
};

const peopleItems: SidebarItem[] = [
  {
    label: "Users",
    icon: UserRoundPlus,
    path: "/admin/users",
  },
  {
    label: "Student Enrollments",
    icon: GraduationCap,
    path: "/admin/student-enrollments",
  },
  {
    label: "Teacher Assignments",
    icon: UsersRound,
    path: "/admin/teacher-assignments",
  },
];

const academicItems: SidebarItem[] = [
  {
    label: "Academic Sessions & Terms",
    icon: CalendarDays,
    path: "/admin/academic-sessions",
  },
  {
    label: "Classes",
    icon: BookOpen,
    path: "/admin/classes",
  },
  {
  label: "Subjects",
  icon: Layers,
  path: "/admin/subjects",
},
{
  label: "Attendance",
  icon: CalendarCheck,
  path: "/admin/attendance",
},
  {
    label: "Results",
    icon: ChartNoAxesColumn,
    path: "/admin/results",
  },
];

function NavigationItem({ label, icon: Icon, path }: SidebarItem) {
  const content = (
    <>
      <Icon className="sidebar-icon" aria-hidden="true" />
      <span className="sidebar-text">{label}</span>
    </>
  );

  if (!path) {
    return (
      <span
        className="sidebar-link sidebar-unavailable"
        aria-disabled="true"
        title={`${label} page coming soon`}
      >
        {content}
      </span>
    );
  }

  return (
    <NavLink
      to={path}
      title={label}
      className={({ isActive }) =>
        `sidebar-link${isActive ? " active" : ""}`
      }
    >
      {content}
    </NavLink>
  );
}

function AdminSidebar({ collapsed }: AdminSidebarProps) {
  const location = useLocation();

  const isDashboard = location.pathname === "/admin/dashboard";

  const [schoolLogo, setSchoolLogo] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;

    async function loadSchoolLogo() {
      try {
        const url = await getMediaObjectUrl(
          "school-logo",
          controller.signal
        );

        if (!active) {
          if (url) URL.revokeObjectURL(url);
          return;
        }

        setSchoolLogo(url);
      } catch (error) {
        if (!controller.signal.aborted) {
          console.error("Unable to load school logo:", error);
        }
      }
    }

    void loadSchoolLogo();

    return () => {
      active = false;
      controller.abort();
    };
  }, []);

  return (
  <aside
    className={`admin-sidebar${collapsed ? " collapsed" : ""}${
      isDashboard ? " sidebar-dashboard-animate" : ""
    }`}
  >
      <div className="sidebar-header">
        <div className="sidebar-brand">
          {schoolLogo ? (
  <img
    src={schoolLogo}
    alt="School logo"
    className="sidebar-brand-logo"
  />
) : (
  <BookOpen
    className="sidebar-brand-icon"
    aria-hidden="true"
  />
)}

          <div className="sidebar-brand-text">
            <h2>BluePrint</h2>
            <span>Group of School</span>
          </div>
        </div>
      </div>

      <nav className="sidebar-nav" aria-label="Admin navigation">
        <NavigationItem
          label="Dashboard"
          icon={House}
          path="/admin/dashboard"
        />

        <div className="sidebar-section">
          <p className="sidebar-section-title">PEOPLE</p>

          {peopleItems.map((item) => (
            <NavigationItem
              key={item.label}
              {...item}
            />
          ))}
        </div>

        <div className="sidebar-section">
          <p className="sidebar-section-title">ACADEMICS</p>

          {academicItems.map((item) => (
            <NavigationItem
              key={item.label}
              {...item}
            />
          ))}
        </div>
      </nav>

      <div className="sidebar-bottom">
        <NavigationItem
          label="Settings"
          icon={Settings}
          path="/admin/settings/account-security"
        />

        <LogoutButton className="sidebar-logout">
          <LogOut
            className="sidebar-icon"
            aria-hidden="true"
          />
          <span className="sidebar-text">Logout</span>
        </LogoutButton>
      </div>
    </aside>
  );
}

export default AdminSidebar;
