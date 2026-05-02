# Release Notes

- Fix: Pro request submission now reaches the billing service — Content-Security-Policy `connect-src` now includes the configured `BILLING_URL`, so the dashboard can call billing cross-origin. Previously `connect-src 'self'` blocked the fetch in the browser before it left.
- Update: replaced `support@`, `privacy@`, `security@agentorbit.tech` contact addresses with `agentorbit.tech@gmail.com` across the dashboard (legal pages, schema.org metadata) and contributor docs.
- Update: session narratives are now terser — 1 short sentence describing the agent's action, plus a second sentence only when anomalies are present.
- Fix: span text passed to the narrative/anomaly LLM is no longer silently cut at 500 bytes from the head. Long inputs/outputs now show head + tail with an explicit `...[truncated, N bytes]...` marker, so end-of-stream errors, refusals, and echo patterns in long responses are visible to the analyzer.
