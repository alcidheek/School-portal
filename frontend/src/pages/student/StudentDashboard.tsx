import LogoutButton from "../../components/LogoutButton";
import { Link } from "react-router-dom";

function StudentDashboard() {
  return <><h1>Student Dashboard</h1><Link to="/student/results">My published results</Link><LogoutButton /></>;
}

export default StudentDashboard;
