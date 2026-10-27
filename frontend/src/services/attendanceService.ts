
import { resultRequest } from "./resultService";

export type AttendanceStatus = "PRESENT" | "ABSENT";

export type AttendanceRecord = {
  id: string;
  student_id: string;
  class_id: string;
  academic_session_id: string;
  term_id: string;
  date: string;
  status: AttendanceStatus;
  recorded_by: string;
  created_at: string;
  updated_at: string;
};

export type AttendanceFilters = {
  class_id?: string;
  academic_session_id?: string;
  term_id?: string;
  date?: string;
  student_id?: string;
};

export type AttendanceResponse = {
  attendance: AttendanceRecord[];
};

export async function getAttendance(
  filters: AttendanceFilters = {}
): Promise<AttendanceRecord[]> {
  const searchParameters = new URLSearchParams();

  if (filters.class_id) {
    searchParameters.set("class_id", filters.class_id);
  }

  if (filters.academic_session_id) {
    searchParameters.set(
      "academic_session_id",
      filters.academic_session_id
    );
  }

  if (filters.term_id) {
    searchParameters.set("term_id", filters.term_id);
  }

  if (filters.date) {
    searchParameters.set("date", filters.date);
  }

  if (filters.student_id) {
    searchParameters.set("student_id", filters.student_id);
  }

  const query = searchParameters.toString();

  const endpoint = query
    ? `admin/attendance?${query}`
    : "admin/attendance";

  const response = await resultRequest<AttendanceResponse>(
    endpoint,
    "GET"
  );

  return response.attendance;
}
