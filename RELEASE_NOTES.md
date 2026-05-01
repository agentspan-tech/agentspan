# Release Notes

- Fix: Pro request submission now reaches the billing service — frontend stopped sending `X-Requested-With` to billing, which billing's CORS preflight did not allow, causing the POST to be blocked by the browser before it ever left.
