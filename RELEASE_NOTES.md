# Release Notes

- Fix: Pro request submission now reaches the billing service — Content-Security-Policy `connect-src` now includes the configured `BILLING_URL`, so the dashboard can call billing cross-origin. Previously `connect-src 'self'` blocked the fetch in the browser before it left.
- Update: replaced `support@`, `privacy@`, `security@agentorbit.tech` contact addresses with `agentorbit.tech@gmail.com` across the dashboard (legal pages, schema.org metadata) and contributor docs.
