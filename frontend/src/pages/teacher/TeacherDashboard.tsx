import LogoutButton from "../../components/LogoutButton";
import { Link } from "react-router-dom";

function TeacherDashboard() {
  return <><h1>Teacher Dashboard</h1><Link to="/teacher/results">Subject results</Link><LogoutButton /></>;
}

export default TeacherDashboard;
