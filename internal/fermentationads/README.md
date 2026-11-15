# Fermentation ADS V1 work in progress

This package is not wired into the application. It must not be enabled until an
independent review, full integration and the commissioning gates in Interfaces
#7 are complete. `Run` requires the caller to fence all UA writers before ADS
handover and to keep them fenced through failure and safe handover. No arbiter
currently enforces that requirement. An error or cancellation stops ADS writes;
it does not command UA ownership or prove physical outputs are safe.

The coordinator sends a disabled image first, then a new enabled command every
200 ms, with a 400 ms deadline for each complete transaction. It permits up to
five seconds for the PLC's post-interlock image to converge while continuing
heartbeats. These bounds are software assumptions based on the PLC source's
one-second watchdog and one-second valve/pump sequencing phases; they need
independent review and timing verification on the actual target. Observations
are PLC-reported images, not physical feedback. Observations may be dropped if
the consumer cannot keep up, so a production alarm/status integration must
separately latch and report terminal errors.

Remaining work includes actual session configuration and lifecycle, an
exclusive UA/ADS ownership arbiter covering all writers and delayed callbacks,
explicit deactivation and UA handover, input scaling and engine integration,
backend/API/UI status and activation, restart/timeout/interlock simulated and
native tests, and independent review. No process activation is configured.
