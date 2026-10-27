-- Keep existing account IDs, credentials and stored email values intact.
-- Abort rather than selecting a winner if legacy emails collide after folding.
LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (
        SELECT lower(btrim(email, E' \t\n\r\f\u000B' || U&'\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000')) FROM users
        GROUP BY lower(btrim(email, E' \t\n\r\f\u000B' || U&'\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000')) HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'Normalized user emails are not unique; review conflicting accounts before applying migration 013';
    END IF;
END $$;

CREATE UNIQUE INDEX users_normalized_email_unique ON users (lower(btrim(email, E' \t\n\r\f\u000B' || U&'\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000')));
