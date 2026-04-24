# Release Notes

- Fix: verification email now sends for Russian-locale users (Subject and From display-name are RFC 2047 encoded). Registration was previously failing with 500 and leaving the account unable to retry.
- Fix: registration rolls back the user row when the verification email fails to send, so the same email can retry instead of hitting a 409.
