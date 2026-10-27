
import { useEffect, useState } from "react";
import { Outlet, useLocation } from "react-router-dom";

import AdminSidebar from "./AdminSidebar";
import AdminTopbar from "./AdminTopbar";
import "./AdminLayout.css";

const PAGE_LOADING_DURATION = 1000;

function AdminLayout() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [loadingPath, setLoadingPath] = useState<string | null>(null);

  const location = useLocation();

  // Dashboard must never display the three-dot loader.
  const isDashboard =
    location.pathname === "/admin/dashboard" ||
    location.pathname === "/admin";

  const toggleSidebar = () => {
    setSidebarCollapsed((current) => !current);
  };

  useEffect(() => {
  if (isDashboard) {
    setLoadingPath(null);
    return;
  }

  setLoadingPath(location.pathname);

  const timeout = window.setTimeout(() => {
    setLoadingPath(null);
  }, PAGE_LOADING_DURATION);

  return () => window.clearTimeout(timeout);
}, [location.pathname, isDashboard]);

  // Derive loading synchronously from the current route.
  // This prevents the destination page from flashing before
  // the loading effect runs.
  const pageLoading =
    !isDashboard && loadingPath !== null
      ? loadingPath === location.pathname
      : false;

  return (
    <div className="admin-layout">
      <AdminSidebar
        collapsed={sidebarCollapsed}
        onToggle={toggleSidebar}
      />

      <div
        className={
          sidebarCollapsed
            ? "admin-main sidebar-collapsed"
            : "admin-main"
        }
      >
        <AdminTopbar
          collapsed={sidebarCollapsed}
          onToggle={toggleSidebar}
        />

        {pageLoading ? (
          <div
            className="admin-page-loader"
            role="status"
            aria-label="Loading page"
          >
            <div className="admin-loading-dots">
              <span />
              <span />
              <span />
            </div>

            <p>Loading page...</p>
          </div>
       ) : (
  <div
    key={location.pathname}
    className={
      isDashboard
        ? "admin-page-content"
        : "admin-page-content admin-page-enter"
    }
  >
    <Outlet />
  </div>
)}
      </div>
    </div>
  );
}

export default AdminLayout;
