# Fermentation ADS V1 work in progress

This package is not wired into the application. It must not be enabled until an
independent review, full integration and the commissioning gates in Interfaces
#7 are complete. `Run` requires the caller to fence all UA writers before ADS
handover and to keep them fenced through failure and safe handover. No arbiter
currently enforces that requirement. An error or cancellation stops ADS writes;
it does not command UA ownership or prove physical outputs are safe.

The caller supplies a positive maximum decision age and must repeatedly publish
newly computed `Decision` values with their local computation timestamps, even
when outputs do not change. A fresh first decision is required before handover.
Stale, future and out-of-order decisions are rejected; if decisions stop, ADS
writes stop when their freshness deadline expires and the PLC watchdog must
then take over. The transaction context is bounded by that deadline. The caller
must choose a limit suitable for its control loop and validate it on the target;
no existing producer is integrated and no production age is approved here.

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

The coordinator closes its link on every exit. A transport operation error,
invalid snapshot or expired context permanently fences the transport and closes
the underlying session; a new session and explicit activation are required.
SDK v3.0.1 `Session.Close` attempts administrative handle release before socket
closure, and a frame already handed to the SDK or network cannot be recalled.
The fence prevents further calls through this transport; it does not prove that
an in-flight commit was not delivered or that the PLC reached SAFE. UA writers
must remain fenced until SAFE is independently confirmed and an approved
handover occurs. Closing may take time; it is not a safety timer or substitute
for the PLC watchdog.

`opcua.Client.FenceWrites` now gates all writes through that client, including
arbitrary API node writes and delayed fermentation callbacks; it waits for
in-flight calls and refuses the handover if any prior UA write returned an
uncertain result. `Supervisor` invokes this one-way fence before ADS commands
and does not release it on error. These primitives are not connected to app
startup or exposed as an activation endpoint. They do not fence another process,
another UA client or an already transmitted frame. A process restart resets the
in-memory fence, so startup ownership reconciliation and independently verified
SAFE handover are mandatory before wiring this into production.

The dedicated inputs are raw PLC `INT` values. The shared contract documents
only “PLC scale”; the existing engine expects floating-point temperatures in
°C. The actual conversion is not specified by this contract or the checked-in
PLC mapping. No scaling is assumed and the engine is not connected to these
inputs until the scale is established and versioned.
