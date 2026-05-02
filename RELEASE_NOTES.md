# Release Notes

- Fix: Pro request submission now reaches the billing service — Content-Security-Policy `connect-src` now includes the configured `BILLING_URL`, so the dashboard can call billing cross-origin. Previously `connect-src 'self'` blocked the fetch in the browser before it left.
