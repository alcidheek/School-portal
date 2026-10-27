
import {
  type AcademicSession,
  type AcademicTerm,
  getAcademicSessions,
  getActiveTerm,
} from "./adminService";

import { resultRequest } from "./resultService";

// Reuse existing academic types and functions.
export type { AcademicSession, AcademicTerm };
export { getAcademicSessions, getActiveTerm };

export type CreateAcademicSessionRequest = {
  name: string;
  start_date: string;
  end_date: string;
  activate: boolean;
  term_name: AcademicTerm["name"];
  term_start_date: string;
  term_end_date: string;
};

export type CreateAcademicTermRequest = {
  term_name: AcademicTerm["name"];
  start_date: string;
  end_date: string;
};

// Retrieve all existing terms for a session.
export async function getAcademicTerms(
  sessionID: string
): Promise<AcademicTerm[]> {
  return resultRequest<AcademicTerm[]>(
    `admin/academic-sessions/${sessionID}/terms`
  );
}

// Create an academic session and its first term.
export async function createAcademicSession(
  data: CreateAcademicSessionRequest
): Promise<AcademicSession> {
  return resultRequest<AcademicSession>(
    "admin/academic-sessions",
    "POST",
    data
  );
}

// Activate an existing academic session.
export async function activateAcademicSession(
  sessionID: string
): Promise<void> {
  await resultRequest<void>(
    `admin/academic-sessions/${sessionID}/activate`,
    "PATCH"
  );
}

// Create a term or activate an existing term.
export async function createOrActivateAcademicTerm(
  sessionID: string,
  data: CreateAcademicTermRequest
): Promise<AcademicTerm> {
  return resultRequest<AcademicTerm>(
    `admin/academic-sessions/${sessionID}/term`,
    "PATCH",
    data
  );
}


 // Delete an unused, inactive academic session.
 export async function deleteAcademicSession(
   sessionID: string
 ): Promise<void> {
   await resultRequest<void>(
     `admin/academic-sessions/${sessionID}`,
     "DELETE"
   );
 }
