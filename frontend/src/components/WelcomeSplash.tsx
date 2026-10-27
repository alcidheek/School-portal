
import { useEffect, useState } from "react";
import LoginPage from "../pages/LoginPage";
import "./WelcomeSplash.css";

const SPLASH_DURATION = 4000;

export default function WelcomeSplash() {
  const [showSplash, setShowSplash] = useState(true);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      setShowSplash(false);
    }, SPLASH_DURATION);

    return () => {
      window.clearTimeout(timeout);
    };
  }, []);

  if (!showSplash) {
    return <LoginPage />;
  }

  return (
    <main className="blueprint-splash">
      <div className="blueprint-splash-content">
        <p className="splash-welcome">Welcome to</p>

        <h1 className="splash-title">BluePrint</h1>

        <p className="splash-subtitle">
          Group of School
        </p>

        <div
          className="splash-progress"
          role="progressbar"
          aria-label="Opening BluePrint"
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div className="splash-progress-fill" />
        </div>

        <p className="splash-caption">
          Preparing your school portal...
        </p>
      </div>
    </main>
  );
}
