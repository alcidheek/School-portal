
import {
  getClasses,
  type SchoolClass,
} from "./adminService";

import { resultRequest } from "./resultService";

// Reuse the existing class type and listing function.
export type { SchoolClass };
export { getClasses };

// Data required to create or update a class.
export type ClassRequest = {
  name: string;
  level: string;
};

// Create a new school class.
export async function createClass(
  data: ClassRequest
): Promise<SchoolClass> {
  return resultRequest<SchoolClass>(
    "admin/classes",
    "POST",
    data
  );
}

// Update an existing class.
export async function updateClass(
  classID: string,
  data: ClassRequest
): Promise<SchoolClass> {
  return resultRequest<SchoolClass>(
    `admin/classes/${classID}`,
    "PATCH",
    data
  );
}

// Delete an unreferenced class.
export async function deleteClass(
  classID: string
): Promise<void> {
  await resultRequest<void>(
    `admin/classes/${classID}`,
    "DELETE"
  );
}
