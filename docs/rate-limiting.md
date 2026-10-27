# Authentication rate limiting

The existing mutex-protected, in-memory token bucket middleware is retained.
Each request spends one token before the handler runs, including successful
authentication, invalid credentials and malformed input. Limits remain:

| Route | Capacity | Refill |
| --- | --- | --- |
| POST `/auth/login` | 5 | 5 tokens/minute |
| POST `/auth/forgot-password` | 3 | 3 tokens/minute |
| POST `/auth/reset-password` | 5 | 5 tokens/minute |

Each route has a distinct limiter, constructed once at router initialization.
Other methods retain the existing JSON 405/Allow behavior; logout and other
application routes do not spend authentication limiter tokens. The optional
`AuthenticationRateLimiters` argument to `SetupRoutes` lets startup share these
same three instances with maintenance, preserving existing constructor callers.

## Bucket behavior

Admission, refill, recent activity and retry calculation are protected by the
same mutex and use one clock sample. Refill is capped at capacity; a backward
clock sample never adds negative tokens or moves refill/activity timestamps back.
`Allow(clientID) bool` and valid `NewRateLimiter(capacity, refillRate)` usage remain
compatible. Constructor misuse fails immediately with a descriptive panic:
capacity/refill must be finite, capacity must support one-token requests and exact
one-token subtraction (1 through 2^53-1), refill must be positive, and total refill
time must be finite. Capacities below one cannot admit a request; huge capacities
that cannot subtract one accurately could otherwise effectively allow unlimited
requests. The three production configurations are unchanged.

HTTP 429 preserves `{"error":"Too many requests. Please try again later."}`.
`Retry-After` is the nonnegative integer number of seconds calculated as
`max(1, ceil((1 - current_tokens) / refill_tokens_per_second))`. It uses the actual
fractional deficit after refill, not a fixed per-endpoint wait, and never exposes
the balance or other client buckets.

## Inactive cleanup and shutdown

`LastSeen` records both admitted and rejected requests. Once a bucket has been
inactive for `capacity / refillRate` seconds, it would be completely full anyway,
so deletion/recreation grants no additional tokens. For all three configured
routes this threshold is 60 seconds. A single process-owned ticker checks all
three maps once per minute, deleting under their existing mutexes. Thus an idle
bucket normally disappears 60-120 seconds after its last request. Recently active
clients, including clients still receiving 429s, are preserved.

No client-specific goroutines and no constructor/router background goroutines are
created. Startup explicitly launches one cleanup loop with the application's
cancellable maintenance context, cancels it on shutdown, and waits for completion
before closing PostgreSQL/exiting. Tests can construct routers without leaking
background loops. No authentication/JWT/reset-code database behavior is changed.

## IP identity and operational limits

The connection's `RemoteAddr` supplies identity. Port differences are ignored;
IPv4, IPv6 and IPv6 zones are parsed, equivalent IPv6 spellings are normalized,
and IPv4-mapped IPv6 addresses share the IPv4 bucket. Malformed synthetic addresses
share a conservative fallback bucket. `X-Forwarded-For` and `X-Real-IP` are ignored.

Behind a reverse proxy, requests therefore share the proxy connection IP's bucket.
Do not start trusting forwarding headers without an explicit trusted-proxy
configuration that validates the immediate peer and the forwarding chain. Prefer
an appropriately configured edge limiter until that policy is implemented. NAT
users may share a public IP and compete for the same tokens.

Buckets are local to one Go process, reset on restart, and are not shared between
instances. Multiple instances can multiply the effective limit; this is not a
distributed abuse-prevention guarantee. Inactive cleanup does not impose a hard
maximum on simultaneous active clients. This middleware complements, and never
replaces, PostgreSQL's persistent reset verification budget and resend cooldown.
No Redis, new dependencies, schema changes or frontend changes are included.

## Verification

Tests use an atomic injectable clock for refill, Retry-After and expiry cases,
concurrent requests/cleanup, and a short cancellable ticker for loop lifecycle.
Route tests cover all three capacities, independent clients/endpoints, malformed
handler outcomes, successful authentication and unaffected logout/method handling.
The existing account-lifecycle HTTP suite uses distinct client IPs to exercise
its account behavior without intentionally exhausting a burst budget.

Changed Go files are formatted. `go test ./... -count=1` and `go vet ./...` passed
with a fresh isolated PostgreSQL 18 cluster on port 55437, database
`rate_isolated_test`; the development database and normal application are not used.
Race detection could not run: CGO is disabled by default, and retrying with
`CGO_ENABLED=1` failed because the required `gcc` C compiler is unavailable.
No commits/pushes or Task 6 work are included.
