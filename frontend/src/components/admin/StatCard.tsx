
import type { LucideIcon } from "lucide-react";
import "./StatCard.css";

type StatCardProps = {
  title: string;
  value: number | string;
  description: string;
  icon: LucideIcon;
  iconClassName: string;
  loading?: boolean;
};

function StatCard({
  title,
  value,
  description,
  icon: Icon,
  iconClassName,
  loading = false,
}: StatCardProps) {
  return (
    <div className="stat-card">
      <div className={`stat-card-icon ${iconClassName}`}>
        <Icon size={28} aria-hidden="true" />
      </div>

      <div className="stat-card-content">
        <p className="stat-title">{title}</p>
        <h3 className="stat-value">
          {loading ? "—" : value}
        </h3>
        <span className="stat-description">
          {loading ? "Loading..." : description}
        </span>
      </div>
    </div>
  );
}

export default StatCard;
