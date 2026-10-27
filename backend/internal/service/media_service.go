package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"

	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
	_ "golang.org/x/image/webp"
)

const MaxMediaBytes = 2 * 1024 * 1024

// Bound decoded memory as well as compressed input (roughly 64 MiB of RGBA).
const maxMediaPixels = 16 * 1024 * 1024

var ErrMediaStorage = errors.New("media storage operation failed")
var ErrMediaCleanup = errors.New("media cleanup pending")

// MediaStorage is implemented by the existing private-bucket MinIOStorage.
type MediaStorage interface {
	Upload(context.Context, string, io.Reader, int64, string) error
	Download(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

type SchoolLogoRepository interface {
	GetLogoObjectKey(context.Context) (*string, error)
	ReplaceLogoObjectKey(context.Context, *string, *string) error
}

type ProfilePictureRepository interface {
	GetProfilePictureObjectKey(context.Context, uuid.UUID) (*string, error)
	ReplaceProfilePictureObjectKey(context.Context, uuid.UUID, *string, *string) error
}

type MediaService struct {
	storage MediaStorage
	school  SchoolLogoRepository
	users   ProfilePictureRepository
}

func NewMediaService(storage MediaStorage, school SchoolLogoRepository, users ProfilePictureRepository) *MediaService {
	return &MediaService{storage: storage, school: school, users: users}
}

type mediaReference struct {
	get    func(context.Context) (*string, error)
	cas    func(context.Context, *string, *string) error
	prefix string
}

func (s *MediaService) logo() mediaReference {
	return mediaReference{s.school.GetLogoObjectKey, s.school.ReplaceLogoObjectKey, "school-logos/"}
}
func (s *MediaService) picture(id uuid.UUID) mediaReference {
	return mediaReference{
		func(ctx context.Context) (*string, error) { return s.users.GetProfilePictureObjectKey(ctx, id) },
		func(ctx context.Context, old, next *string) error {
			return s.users.ReplaceProfilePictureObjectKey(ctx, id, old, next)
		},
		"profile-pictures/" + id.String() + "/",
	}
}

// Upload methods also replace existing images. Authorization belongs to the caller.
func (s *MediaService) UploadSchoolLogo(ctx context.Context, input io.Reader) error {
	return s.replace(ctx, s.logo(), input)
}
func (s *MediaService) UploadProfilePicture(ctx context.Context, id uuid.UUID, input io.Reader) error {
	if id == uuid.Nil {
		return ErrInvalidInput
	}
	return s.replace(ctx, s.picture(id), input)
}
func (s *MediaService) DeleteSchoolLogo(ctx context.Context) error {
	return s.remove(ctx, s.logo())
}
func (s *MediaService) DeleteProfilePicture(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidInput
	}
	return s.remove(ctx, s.picture(id))
}
func (s *MediaService) DownloadSchoolLogo(ctx context.Context) (io.ReadCloser, error) {
	return s.download(ctx, s.logo())
}
func (s *MediaService) DownloadProfilePicture(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.download(ctx, s.picture(id))
}

// MediaCleanupError contains no public object identifier or backend error text.
// Retry with a fresh context after cancellation and after the failed database
// statement is known to have settled (it may still commit after a lost connection).
// Persisting a durable cleanup job
// requires a future outbox/worker; this retry handle lasts only in this process.
type MediaCleanupError struct {
	storage   MediaStorage
	ref       mediaReference
	key       string
	previous  *string
	committed bool
}

func (e *MediaCleanupError) Error() string {
	return "media cleanup pending; database outcome must be checked before deletion"
}
func (e *MediaCleanupError) Is(target error) bool   { return target == ErrMediaCleanup }
func (e *MediaCleanupError) ReferenceUpdated() bool { return e.committed }
func (e *MediaCleanupError) Retry(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errors.Join(e, err)
	}
	current, err := e.ref.get(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.Join(e, mediaError(ctx, err))
	}
	// Only fresh random keys can be installed by this service; old keys are
	// never restored or shared across owners. Thus an unreferenced key cannot
	// become referenced again through a concurrent service call.
	keys := []string{e.key}
	if e.previous != nil {
		keys = append(keys, *e.previous)
	}
	for _, key := range keys {
		if current != nil && *current == key {
			continue
		}
		if err := e.storage.Delete(ctx, key); err != nil {
			return errors.Join(e, mediaError(ctx, err))
		}
	}
	return nil
}

func mediaError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if errors.Is(err, repository.ErrMediaObjectConflict) {
		return ErrConflict
	}
	return ErrMediaStorage
}

func (s *MediaService) cleanup(ctx context.Context, ref mediaReference, key string, committed bool) error {
	e := &MediaCleanupError{storage: s.storage, ref: ref, key: key, committed: committed}
	return e.Retry(ctx)
}

func (s *MediaService) replace(ctx context.Context, ref mediaReference, input io.Reader) error {
	data, mime, err := validateMedia(ctx, input)
	if err != nil {
		return err
	}
	old, err := ref.get(ctx)
	if err != nil {
		return mediaError(ctx, err)
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return ErrMediaStorage
	}
	key := ref.prefix + id.String()
	if err := s.storage.Upload(ctx, key, bytes.NewReader(data), int64(len(data)), mime); err != nil {
		return errors.Join(mediaError(ctx, err), s.cleanup(ctx, ref, key, false))
	}
	if err := ref.cas(ctx, old, &key); err != nil {
		if errors.Is(err, repository.ErrMediaObjectConflict) {
			return errors.Join(ErrConflict, s.cleanup(ctx, ref, key, false))
		}
		var postgresError *pq.Error
		if errors.As(err, &postgresError) {
			// A PostgreSQL ErrorResponse confirms the statement failed.
			return errors.Join(mediaError(ctx, err), s.cleanup(ctx, ref, key, false))
		}
		// An arbitrary SQL/transport error does not prove rollback: the statement
		// could still commit after a lost connection. Never delete that upload
		// automatically; return explicit reconciliation work instead.
		return errors.Join(mediaError(ctx, err), &MediaCleanupError{storage: s.storage, ref: ref, key: key, previous: copyMediaKey(old)})
	}
	if old != nil {
		return s.cleanup(ctx, ref, *old, true)
	}
	return nil
}

func copyMediaKey(key *string) *string {
	if key == nil {
		return nil
	}
	value := *key
	return &value
}

func (s *MediaService) remove(ctx context.Context, ref mediaReference) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	old, err := ref.get(ctx)
	if err != nil {
		return mediaError(ctx, err)
	}
	if old == nil {
		return nil
	}
	if err := ref.cas(ctx, old, nil); err != nil {
		// Include cleanup work for an ambiguous commit; a known CAS conflict
		// belongs to the winner and must not delete its previous object.
		if errors.Is(err, repository.ErrMediaObjectConflict) {
			return ErrConflict
		}
		return errors.Join(mediaError(ctx, err), &MediaCleanupError{storage: s.storage, ref: ref, key: *old})
	}
	return s.cleanup(ctx, ref, *old, true)
}

func (s *MediaService) download(ctx context.Context, ref mediaReference) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := ref.get(ctx)
	if err != nil {
		return nil, mediaError(ctx, err)
	}
	if key == nil {
		return nil, ErrNotFound
	}
	r, err := s.storage.Download(ctx, *key)
	if err != nil {
		return nil, mediaError(ctx, err)
	}
	return r, nil
}

type mediaContextReader struct {
	ctx    context.Context
	reader io.Reader
}

type mediaImageWriter struct {
	ctx    context.Context
	buffer bytes.Buffer
}

func (w *mediaImageWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > MaxMediaBytes-w.buffer.Len() {
		return 0, fmt.Errorf("%w: normalized image exceeds 2 MiB", ErrInvalidInput)
	}
	return w.buffer.Write(p)
}

func (r mediaContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func validateMedia(ctx context.Context, input io.Reader) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if input == nil {
		return nil, "", ErrInvalidInput
	}
	data, err := io.ReadAll(io.LimitReader(mediaContextReader{ctx, input}, MaxMediaBytes+1))
	if err != nil {
		return nil, "", mediaError(ctx, err)
	}
	if len(data) == 0 || len(data) > MaxMediaBytes {
		return nil, "", fmt.Errorf("%w: image must contain 1 to %d bytes", ErrInvalidInput, MaxMediaBytes)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	mime := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "webp": "image/webp"}[format]
	if err != nil || mime == "" || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxMediaPixels {
		return nil, "", fmt.Errorf("%w: valid PNG, JPEG or WebP image within pixel limits required", ErrInvalidInput)
	}
	// Decode all pixels: header sniffing alone accepts truncated/corrupt images.
	decoded, _, err := image.Decode(mediaContextReader{ctx, bytes.NewReader(data)})
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", fmt.Errorf("%w: corrupt image", ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	// Store only canonical pixel content, stripping metadata and trailing payloads.
	// WebP and JPEG input are normalized to PNG as well; no WebP encoder is needed.
	normalized := mediaImageWriter{ctx: ctx}
	if err := png.Encode(&normalized, decoded); err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if errors.Is(err, ErrInvalidInput) {
			return nil, "", err
		}
		return nil, "", ErrMediaStorage
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return normalized.buffer.Bytes(), "image/png", nil
}
