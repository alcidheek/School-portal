import { authenticatedFetch } from "./authSession.ts";
import { readApiResponse } from "./apiResponse.ts";

// User information returned by the backend.
export type AdminUser = {
  id: string;
  full_name: string;
  email: string;
  phone: string;
  role: "ADMIN" | "TEACHER" | "STUDENT";

  admission_number?: string | null;
  employee_number?: string | null;

  guardian_name?: string | null;
  guardian_phone?: string | null;

  is_active: boolean;

  created_at: string;
  updated_at: string;
};

// Shape returned by GET /api/v1/admin/users.
type UsersResponse = {
  users: AdminUser[];
};

// Retrieve all users for the admin.
export async function getUsers(): Promise<AdminUser[]> {
  // Get the JWT saved during login.
  const token = localStorage.getItem("token");

  const response = await authenticatedFetch(
    "http://localhost:8080/api/v1/admin/users",
    {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  );

  const result = await readApiResponse<UsersResponse>(response, "Failed to fetch users");

  return result.users;
}

// School class returned by the backend.
export type SchoolClass = {
  id: string;
  name: string;
  level: string;
  created_at: string;
  updated_at: string;
};

// Subject returned by the backend.
export type Subject = {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
};

// Retrieve all school classes.
export async function getClasses(): Promise<SchoolClass[]> {
  // Get the admin JWT saved during login.
  const token = localStorage.getItem("token");

  const response = await authenticatedFetch(
    "http://localhost:8080/api/v1/admin/classes",
    {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  );

  // Backend returns the array directly.
  return readApiResponse<SchoolClass[]>(response, "Failed to fetch classes");
}

// Retrieve all school subjects.
export async function getSubjects(): Promise<Subject[]> {
  // Get the admin JWT saved during login.
  const token = localStorage.getItem("token");

  const response = await authenticatedFetch(
    "http://localhost:8080/api/v1/admin/subjects",
    {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  );

  // Backend returns the array directly.
  return readApiResponse<Subject[]>(response, "Failed to fetch subjects");
}

// Academic session returned by the backend.
export type AcademicSession = {
  id: string;
  name: string;
  start_date: string;
  end_date: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
};

// Retrieve all academic sessions.
export async function getAcademicSessions(): Promise<AcademicSession[]> {
  // Get the admin JWT saved during login.
  const token = localStorage.getItem("token");

  const response = await authenticatedFetch(
    "http://localhost:8080/api/v1/admin/academic-sessions",
    {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  );

  // Backend returns the sessions array directly.
  return readApiResponse<AcademicSession[]>(response, "Failed to fetch academic sessions");
}

// Term returned by the backend.
export type AcademicTerm = {
  id: string;
  academic_session_id: string;
  name: "FIRST_TERM" | "SECOND_TERM" | "THIRD_TERM";
  start_date: string;
  end_date: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
};

// Get the active term for a specific academic session.
export async function getActiveTerm(
  sessionID: string
): Promise<AcademicTerm> {
  const token = localStorage.getItem("token");

  const response = await authenticatedFetch(
    `http://localhost:8080/api/v1/admin/academic-sessions/${sessionID}/active-term`,
    {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  );

  return readApiResponse<AcademicTerm>(response, "Failed to fetch active term");
}

// Data required when the admin creates a user.
export type CreateUserRequest = {
  full_name: string;
  email: string;
  phone: string;
  password: string;
  role: "STUDENT" | "TEACHER";

  // Only students use these fields.
  guardian_name?: string;
  guardian_phone?: string;
};

// Response returned after the user is created.
export type CreateUserResponse = {
  message: string;
  admission_number?: string;
  employee_number?: string;
};

// Create a new student or teacher account.
export async function createUser(
  data: CreateUserRequest
): Promise<CreateUserResponse> {
  // Registration is admin-protected, so send the JWT.
  const token = localStorage.getItem("token");

  const response = await authenticatedFetch(
    "http://localhost:8080/auth/register",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(data),
    }
  );

  return readApiResponse<CreateUserResponse>(response, "Failed to create user");
}


/* =========================================
   SCHOOL PROFILE
========================================= */

// School information returned by the backend.
export type SchoolProfile = {
  id: number;
  name: string;
  motto: string;
  email: string;
  phone: string;
  address: string;
  website: string;
  created_at: string;
  updated_at: string;
};

// Fields administrators can edit.
export type UpdateSchoolProfileRequest = Pick<
  SchoolProfile,
  "name" | "motto" | "email" | "phone" | "address" | "website"
>;

// Retrieve the school's saved information.
export async function getSchoolProfile(): Promise<SchoolProfile | null> {
  const token = localStorage.getItem("token");

  const response = await authenticatedFetch(
    "http://localhost:8080/api/v1/admin/school-profile",
    {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  );

  // No profile has been created yet.
  if (response.status === 404) {
    return null;
  }

  return readApiResponse<SchoolProfile>(
    response,
    "Failed to fetch school profile"
  );
}

// Create or update the school's information.
export async function updateSchoolProfile(
  data: UpdateSchoolProfileRequest
): Promise<SchoolProfile> {
  const token = localStorage.getItem("token");

  const response = await authenticatedFetch(
    "http://localhost:8080/api/v1/admin/school-profile",
    {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(data),
    }
  );

  return readApiResponse<SchoolProfile>(
    response,
    "Failed to update school profile"
  );
}
