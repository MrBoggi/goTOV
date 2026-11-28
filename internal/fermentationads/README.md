# Fermentation ADS V1 work in progress

This package is not wired into the application. It must not be enabled until an
independent review, full integration and the commissioning gates in Interfaces
#7 are complete. The draft supervisor fences the configured UA client before ADS startup, but
no app-integrated arbiter enforces exclusivity across all writers and restarts. An error or cancellation stops ADS writes;
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
200 ms, with a 400 ms deadline covering the pre-write check, write and
acknowledgement. Before each enabled commit it rechecks ownership, session,
command validity, watchdog and applied image for the preceding command. Once
the disabled acknowledgement is established, a fault or unexpected identity
ends the run even if the new command is still awaiting acknowledgement. An
applied heating/cooling conflict or pump running without a valve ends the run
immediately. Interlock-consistent image differences may converge within five
seconds while heartbeats continue. These bounds are software assumptions based on the PLC source's
one-second watchdog and one-second valve/pump sequencing phases; they need
independent review and timing verification on the actual target. Observations
are PLC-reported images, not physical feedback. Observations may be dropped if
the consumer cannot keep up, so a production alarm/status integration must
separately latch and report terminal errors.

Offline tests inject fault, owner and watchdog loss, cleared or foreign
sessions, and hazardous applied images. The transport simulation delivers a
commit but drops its reply and verifies the coordinator stops, retains the UA
fence and refuses reuse. A distinct explicit activation in an independently
reconciled simulation starts with a new session and a disabled command. These
tests do not provide a real reconciliation mechanism or establish PLC timing.

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

`opcua.Client.BeginFence` fences the five fermentation outputs and all
unclassified UA nodes through that client, including generic API writes and
delayed fermentation callbacks. It permits only the explicitly listed 14
non-fermentation outputs from the checked-in PLC source while fenced. Unknown
identities and aliases are denied. It waits within the caller’s context for
in-flight potentially relevant calls and refuses handover if an outcome is
uncertain. The list and its physical independence must be checked on the actual
PLC target before use; unclassified writes remain unavailable while fenced.
`Supervisor` claims a token before opening any ADS session. On a pre-ADS
failure it gives up ADS start and can release the fence only if relevant UA
writes have drained with known successful outcomes. It reports UNKNOWN and
keeps the fence if those conditions cannot be established. Once ADS start has
been reserved it never automatically releases the fence. These primitives are
not connected to app startup or exposed as an activation endpoint. They do not fence another process,
another UA client or an already transmitted frame. A process restart resets the
in-memory fence, so startup ownership reconciliation and independently verified
SAFE handover are mandatory before wiring this into production.

The dedicated inputs are raw PLC `INT` values. The shared contract documents
only “PLC scale”; the existing engine expects floating-point temperatures in
°C. The actual conversion is not specified by this contract or the checked-in
PLC mapping. No scaling is assumed and the engine is not connected to these
inputs until the scale is established and versioned.

## Unresolved handover safety blocker

The supervisor must not be connected to application activation in its current
form. The UA fence is established before the first disabled ADS command is
confirmed. If the decision stream is absent or expires, or the ADS connection
fails or a commit's delivery is uncertain, the PLC may still own the outputs in
UA mode with a previously active UA output. The fence then prevents the existing
UA regulator from changing that output. The PLC's ADS watchdog only protects
ADS ownership and does not prove that this UA-owned output is off. Conversely,
releasing the UA fence automatically is unsafe if an in-flight ADS command could
still acquire ownership or the target's state is unknown. Even a successful
local context cancellation or SDK close cannot establish which command reached
the PLC.

A complete handover needs a reviewed protocol and integration that can verify
and maintain the intended UA or PLC-safe state across this interval, deal with
ambiguous/in-flight writes and process restarts, and keep the original regulator
able to operate until the safe ownership transition is actually established.
The target's reported owner, watchdog and applied image must be corroborated as
required by commissioning; the applied image is not physical feedback. The
current contract and available runtime evidence do not establish that recovery
protocol. Do not auto-release the fence, infer physical SAFE from a timeout, or
turn this draft into a production activation path.

## Pre-ADS abort transaction

A timed-out fence may be aborted only with its matching transaction token,
before ADS startup is irrevocably reserved, after all relevant in-flight UA
writes return successfully. Abort and ADS reservation share a lock so they
cannot both win; a stale token cannot alter a later fence. Supervisor must own
ADS access exclusively, and its link factory must create a fresh session only
when invoked after reservation. Other ADS clients, a previously active session,
a factory with earlier side effects, or frames outside this control invalidate
that assumption. The primitive cannot prove their absence. Once ADS startup is
reserved, failed connect, ambiguous commit or timeout leaves UA fenced and
requires independent reconciliation. A successful pre-ADS abort restores the
existing UA path only; it is not evidence that any physical output is SAFE.
The unresolved handover blocker above remains for failures after reservation.

The read-only `/api/fermentation/control-status` endpoint reports the current
backend wiring as UA default, dedicated supervisor not started, unknown PLC
owner/watchdog and ADS selection unavailable. It does not observe this draft
supervisor or claim physical SAFE. Its response follows the proposed
`goTOV-Interfaces/api/fermentation-control-status/v1` contract and must be
reviewed and merged after that contract. A future activation implementation
must revise the contract and status wiring together; it must never retain this
static status when a supervisor is actually started.

The intent-only `GET/POST /api/fermentation/control-path` resource is distinct
from this adapter. It persists requested path and idempotency keys in the
fermentation SQLite database but has no PLC-writing or auto-resume path. UA is
reported connected only when its client reports connected and a bounded
read-only request to standard OPC UA ServerStatus.State confirms a running
server. The request has a two-second deadline; failure or timeout returns
unknown, and a disconnected client is reported disconnected. This probe
verifies a transport/session at that moment, not fermentation ownership or
physical feedback. The dedicated ADS transport is not wired into the application and is unknown,
even if the separate global input ADS monitor is healthy. These are transport
offers, not verified PLC owner, physical feedback or a commissioning decision.
Accepted requests are durably marked unknown with a high alert because the
required handover is unavailable. Exact retries return their original acceptance
and never trigger a PLC write; idempotency records are retained without expiry.
