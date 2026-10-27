
import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
} from "react";

import { UserRound, Camera, Trash2 } from "lucide-react";

import {
  getMediaObjectUrl,
  uploadMedia,
  deleteMedia,
  validateImage,
} from "../../services/mediaService.ts";

export default function ProfilePictureSettings() {
  const inputRef = useRef<HTMLInputElement>(null);
  const imageUrlRef = useRef<string | null>(null);
  const mountedRef = useRef(false);
  const busyRef = useRef(false);

  const [imageUrl, setImageUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  function replaceImageUrl(nextUrl: string | null) {
    if (!mountedRef.current) {
      if (nextUrl) URL.revokeObjectURL(nextUrl);
      return;
    }

    const previousUrl = imageUrlRef.current;

    imageUrlRef.current = nextUrl;
    setImageUrl(nextUrl);

    if (previousUrl && previousUrl !== nextUrl) {
      URL.revokeObjectURL(previousUrl);
    }
  }

  useEffect(() => {
    mountedRef.current = true;
    const controller = new AbortController();

    async function loadPicture() {
      try {
        const url = await getMediaObjectUrl(
          "profile-picture",
          controller.signal
        );

        if (controller.signal.aborted) {
          if (url) URL.revokeObjectURL(url);
          return;
        }

        replaceImageUrl(url);
      } catch (err) {
        if (!controller.signal.aborted) {
          setError(
            err instanceof Error
              ? err.message
              : "Unable to load profile picture."
          );
        }
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      }
    }

    void loadPicture();

    return () => {
      mountedRef.current = false;
      controller.abort();

      if (imageUrlRef.current) {
        URL.revokeObjectURL(imageUrlRef.current);
        imageUrlRef.current = null;
      }
    };
  }, []);

  async function handleUpload(
    event: ChangeEvent<HTMLInputElement>
  ) {
    const file = event.target.files?.[0];

    if (!file || busyRef.current || loading) return;

    event.target.value = "";

    const validationError = validateImage(file);

    if (validationError) {
      setError(validationError);
      setSuccess("");
      return;
    }

    busyRef.current = true;
    setBusy(true);
    setError("");
    setSuccess("");

    let uploaded = false;

    try {
      await uploadMedia("profile-picture", file);
      uploaded = true;

      const updatedUrl = await getMediaObjectUrl(
        "profile-picture"
      );

      replaceImageUrl(updatedUrl);

      if (mountedRef.current) {
        setSuccess("Profile picture updated successfully.");
        window.dispatchEvent(
          new Event("blueprint:profile-picture-updated")
        );
      }
    } catch (err) {
      if (mountedRef.current) {
        setError(
          uploaded
            ? "Picture saved, but the preview could not be refreshed. Reload the page."
            : err instanceof Error
              ? err.message
              : "Failed to upload profile picture."
        );
      }
    } finally {
      busyRef.current = false;

      if (mountedRef.current) {
        setBusy(false);
      }
    }
  }

  async function handleRemove() {
    if (busyRef.current || loading) return;

    busyRef.current = true;
    setBusy(true);
    setError("");
    setSuccess("");

    try {
      await deleteMedia("profile-picture");

      replaceImageUrl(null);

      if (mountedRef.current) {
        setSuccess("Profile picture removed successfully.");
        window.dispatchEvent(
          new Event("blueprint:profile-picture-updated")
        );
      }
    } catch (err) {
      if (mountedRef.current) {
        setError(
          err instanceof Error
            ? err.message
            : "Failed to remove profile picture."
        );
      }
    } finally {
      busyRef.current = false;

      if (mountedRef.current) {
        setBusy(false);
      }
    }
  }

  return (
    <section className="security-card profile-picture-card">
      <h2>Profile Picture</h2>
      <p>
        Add a profile photo to personalize your school portal
        account.
      </p>

      <div className="profile-picture-content">
        <div className="profile-picture-preview">
          {imageUrl ? (
            <img src={imageUrl} alt="Your profile" />
          ) : (
            <UserRound size={48} aria-hidden="true" />
          )}
        </div>

        <div className="profile-picture-details">
          <strong>
            {loading
              ? "Loading picture..."
              : imageUrl
                ? "Your profile picture"
                : "No profile picture yet"}
          </strong>

          <p>PNG, JPEG or WebP. Maximum 2 MiB.</p>

          <input
            ref={inputRef}
            type="file"
            accept="image/png,image/jpeg,image/webp"
            onChange={handleUpload}
            style={{ display: "none" }}
            aria-label="Select profile picture"
          />

          <div className="profile-picture-actions">
            <button
              type="button"
              disabled={busy || loading}
              onClick={() => inputRef.current?.click()}
            >
              <Camera size={17} />
              {busy
                ? "Processing..."
                : imageUrl
                  ? "Change Picture"
                  : "Upload Picture"}
            </button>

            {imageUrl && (
              <button
                type="button"
                className="profile-picture-remove"
                onClick={handleRemove}
                disabled={busy || loading}
              >
                <Trash2 size={17} />
                Remove
              </button>
            )}
          </div>
        </div>
      </div>

      {error && <p role="alert">{error}</p>}

      {success && (
        <p className="security-success" role="status">
          {success}
        </p>
      )}
    </section>
  );
}
