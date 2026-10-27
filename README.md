# School-portal

## Password reset

Add these values to your local `.env` (use your Gmail address and Gmail app password):

```dotenv
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_EMAIL=your-account@gmail.com
SMTP_APP_PASSWORD=your-gmail-app-password
```

SMTP settings are required at startup. Port 587 uses STARTTLS; port 465 uses
implicit TLS. Credentials are loaded from the environment, never from source code.
Migration `009_password_reset_codes.sql` runs automatically on server startup.

Send `POST /auth/forgot-password` with JSON:

```json
{"email":"student@example.com"}
```

Send `POST /auth/reset-password` with JSON:

```json
{"email":"student@example.com","code":"012345","new_password":"new-password"}
```

Keep the code as a string to preserve leading zeroes. Codes expire after 15 minutes.
The endpoint also accepts `reset_code` in place of `code`. Surrounding code
whitespace is trimmed; if both fields are supplied, their values must match.
Passwords follow the existing minimum of 6 characters and bcrypt's maximum of
72 bytes. Successful resets invalidate all outstanding codes for the account.
Unknown/inactive accounts receive the same forgot-password success message.
Invalid input or codes return 400; database/SMTP failures return a generic 500.

Run `go test ./...`. Set `TEST_DATABASE_URL` to a PostgreSQL test database to also
run the transaction, expiry, reuse, account binding, and rollback tests; these use
temporary tables. No real emails are sent by the tests.
