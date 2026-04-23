import { ApiError } from './api'
import type { Translations } from '@/i18n/en'

/**
 * Map a thrown error to a localized user-facing message. Known backend error
 * codes (from ApiError.code) get specific strings; anything else falls back
 * to the generic "something went wrong" login error.
 */
export function apiErrorMessage(err: unknown, t: Translations): string {
  if (!(err instanceof ApiError)) return t.auth_login_error
  switch (err.code) {
    case 'email_not_verified':
      return t.auth_error_email_not_verified
    case 'invalid_credentials':
      return t.auth_login_invalid
    case 'email_exists':
      return t.auth_email_exists
    case 'registration_closed':
      return t.auth_error_registration_closed
    case 'rate_limit_exceeded':
      return t.auth_error_rate_limited
    case 'invalid_token':
    case 'token_expired':
      return t.auth_error_invalid_token
    default:
      return t.auth_login_error
  }
}
