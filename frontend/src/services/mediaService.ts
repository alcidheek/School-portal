
import { authenticatedFetch } from "./authSession.ts";
import { readApiResponse } from "./apiResponse.ts";

const API_BASE = "http://localhost:8080/api/v1";

export const MAX_IMAGE_SIZE = 2 * 1024 * 1024;

export type MediaKind = "school-logo" | "profile-picture";

export type MediaUploadResponse = {
  url: string;
};

const MEDIA_PATHS: Record<MediaKind, {
  read: string;
  write: string;
}> = {
  "school-logo": {
    read: "/school-profile/logo",
    write: "/admin/school-profile/logo",
  },
  "profile-picture": {
    read: "/users/me/profile-picture",
    write: "/users/me/profile-picture",
  },
};

function authHeaders(): HeadersInit {
  const token = localStorage.getItem("token");

  return {
    Authorization: `Bearer ${token}`,
  };
}

export function validateImage(file: File): string | null {
  const allowedTypes = [
    "image/png",
    "image/jpeg",
    "image/webp",
  ];

  if (!allowedTypes.includes(file.type)) {
    return "Only PNG, JPEG and WebP images are supported.";
  }

  if (file.size > MAX_IMAGE_SIZE) {
    return "Image must not exceed 2 MB.";
  }

  if (file.size === 0) {
    return "Please select a non-empty image.";
  }

  return null;
}

// Upload or replace an image.
export async function uploadMedia(
  kind: MediaKind,
  file: File
): Promise<MediaUploadResponse> {
  const validationError = validateImage(file);

  if (validationError) {
    throw new Error(validationError);
  }

  const form = new FormData();
  form.append("file", file);

  const response = await authenticatedFetch(
    `${API_BASE}${MEDIA_PATHS[kind].write}`,
    {
      method: "PUT",
      headers: authHeaders(),
      body: form,
    }
  );

  // Do not manually set Content-Type for FormData.
  return readApiResponse<MediaUploadResponse>(
    response,
    "Failed to upload image"
  );
}

// Retrieve a protected image as a browser object URL.
export async function getMediaObjectUrl(
  kind: MediaKind,
  signal?: AbortSignal
): Promise<string | null> {
  const response = await authenticatedFetch(
    `${API_BASE}${MEDIA_PATHS[kind].read}`,
    {
      method: "GET",
      headers: authHeaders(),
      signal,
    }
  );

  if (response.status === 404) {
    return null;
  }

  if (!response.ok) {
    await readApiResponse(response, "Failed to load image");
    throw new Error("Failed to load image");
  }

  const blob = await response.blob();

  return URL.createObjectURL(blob);
}

// Delete the current image.
export async function deleteMedia(
  kind: MediaKind
): Promise<void> {
  const response = await authenticatedFetch(
    `${API_BASE}${MEDIA_PATHS[kind].write}`,
    {
      method: "DELETE",
      headers: authHeaders(),
    }
  );

  if (!response.ok) {
    await readApiResponse(response, "Failed to delete image");
    throw new Error("Failed to delete image");
  }
}
