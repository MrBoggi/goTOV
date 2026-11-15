# ADS V1 input diagnostics — Interfaces #6

This standalone command reuses the #72 reader and its fail-closed V1 decoder.
It does not start `cmd/gotov`, `app.RunServer`, the control engine, HTTP server,
OPC UA production client, fermentation controller or database. The #73 status
API remains unchanged; it is not a source of independent input values.
No application config, `.env`, credentials or secret files are loaded.

## Acceptance and actual gaps

Before this change, the existing opt-in test checked one snapshot without
reporting metadata or values. The #4 transport tests already covered the SDK
service allowlist and cleanup. Missing pieces were evidence output, distinct
cycle collection, and an independent OPC UA/XAE comparison with complete fields.

The new command outputs symbol name, reported datatype and byte size, each
validated input value, even publisher sequence and UTC read window. It requires
at least three distinct forward sequences; repeated reads of one cycle do not
count. Sequence wrap is allowed; regressions/restarts fail. Transport, contract,
stability, timeout and cleanup failures return nonzero and retain partial JSON
with `Error`. Partial evidence must never be accepted as a successful capture.
No retry hides an invalid snapshot. A rapidly changing publisher may cause the
three-read stability check to reject a capture: record that rejection and
investigate timing separately, without relaxing the contract.

Comparison is offline against an independently captured XAE/OPC UA reference.
It checks every raw INT, REAL and BOOL exactly, matching even sequence, target,
sample count and bounded UTC windows. No unit scaling or floating tolerance is
inferred. Missing zero/false fields, nulls, incomplete captures, stale windows,
wrong contract, duplicate cycles and value mismatches fail closed.

## Build and offline demonstration

From goTOV, build only this diagnostic binary:

```sh
go build -o /tmp/gotov-ads-diagnostic ./cmd/ads-diagnostic
/tmp/gotov-ads-diagnostic \
  --capture-file cmd/ads-diagnostic/testdata/ads.json \
  --reference-file cmd/ads-diagnostic/testdata/xae.json --max-skew 1s
```

The checked-in fixtures derive from the canonical V1 vector, using three
invented sequences. They demonstrate the command and are **synthetic, not live
evidence**. Default invocation without `--live` and without both offline paths
returns an error before creating a client or opening a network connection.

## Live capture with an already known target

Supply all seven confirmed network fields; there are no implicit host, NetID,
AMS port or TCP port choices. Do not infer them from examples or search secrets.
The operator must confirm the target/runtime and existing route independently.
No new credentials are needed or accepted by this command.

```sh
/tmp/gotov-ads-diagnostic --live \
  --host "$CONFIRMED_ADS_HOST" --port "$CONFIRMED_ADS_TCP_PORT" \
  --target-net-id "$CONFIRMED_TARGET_NET_ID" --target-port "$CONFIRMED_RUNTIME_PORT" \
  --local-net-id "$CONFIRMED_LOCAL_NET_ID" --local-port "$CONFIRMED_LOCAL_AMS_PORT" \
  --cycles 5 --interval 1s --timeout 60s > ads-capture.json
```

Check exit status and `Error`, not just the presence of a file. Target in the
artifact is `target-net-id:target-port`. The read window is client UTC, not a PLC
timestamp. Clock synchronization and cycle provenance must be documented.
The command permits only the existing input client operations: Connect,
ReadState, input symbol lookup, three full handle reads per snapshot and Close.
SDK symbol ReadWrite and handle-release Write are administrative operations;
`WithSkipRouteRegistration()` remains enabled. The SDK also attempts read-only
UDP Identify. No process/output/commit/watchdog writes or route changes occur.

## Independent XAE/OPC UA reference

Prepare `reference.json` with the same structure as the synthetic XAE fixture:

- `Source`: `xae` or `opcua`; `Target`: the independently confirmed same runtime.
- `ReferenceEvidence`: independent source capture artifact identifier, UTC clock
  basis, node/symbol mapping and how each publisher cycle was correlated.
- `Symbol`: `Name` = `GVL_ADS.stInputs`, actual `Size` = 52 and reported datatype.
- `Samples`: one complete independent observation for each ADS capture sequence,
  in forward order, with UTC `StartedAt`, `FinishedAt` and every `Inputs` field.
- `Error`: empty only for a complete successful independent acquisition.

Observe the existing legacy UA/fbUA values through an already available read-only
XAE/OPC UA acquisition. Correlate them with the publisher's stable even sequence;
record sequence before and after the independent read and reject observations
that cross a publisher cycle. A supported independent trace with demonstrated
same-cycle correlation can also supply the reference. Merely collecting similar
wall-clock readings, observing unchanged sensor values, or copying ADS inputs
into the reference does not establish independent comparison. Do not pause the
PLC, add PLC code, enable outputs or start the goTOV app to obtain evidence.
If a reference cannot establish cycle correlation, report it as unavailable.

All INT names map directly to the contract's PLC `i` field (e.g. `HltTemp` ↔
`iHltTemp`, `PhValue` ↔ `iPhValue`, `GlycolChillerTemp` ↔ `iGlycolChillerTemp`).
`FlowHLT`, `FlowMLT`, `FlowWort`, `MltRecircTemp` map to their `r` fields;
the three `OutOfRange` fields map to their `b` fields. `Version`, `StructureSize`
and `Sequence` are independently observed publisher headers, not UA sensor data.
Use the exact PLC raw representation from `ads/v1/go/layout.md`; verify actual
legacy node/symbol mappings on the target rather than assuming NodeIDs or units.

```sh
/tmp/gotov-ads-diagnostic --capture-file ads-capture.json \
  --reference-file reference.json --max-skew 1s > comparison.json
```

Choose and justify `max-skew` from the acquisition method; it bounds each read
window and cross-source start difference. It does not prove cycle identity.
Success means exact agreement for those recorded samples, not a commissioning,
output-safety or production-cutover approval. Comparison fails on the first
mismatch, identifying sample/sequence; inspect both retained artifacts for values.
The software cannot authenticate a manually supplied reference: preserve and
review its independent source artifact and correlation evidence separately.

## Evidence template

```text
Issue: goTOV-Interfaces#6
Operator/reviewer, date, Go/Interfaces branches and HEADs:
Local diff (include untracked files), contract/version/vector:
Target/runtime and already established route confirmed by:
Diagnostic binary revision/hash, exact invocation, exit status:
ADS capture artifact/hash: metadata; complete/partial; cycles/sequences; UTC range:
Independent source: XAE or OPC UA; artifact/hash; actual legacy mappings:
Cycle correlation method and raw pre/post sequences:
UTC clock basis, chosen max-skew and reason:
Comparison invocation, exit status, matched cycles or mismatch details:
Local: tests/vet/build/lint/container build and baseline failures:
Live: executed observations and rejected attempts (or UNVERIFIED, reason):
Unknown: unobserved behaviour, unavailable target/config/reference:
Independent review conclusion and remaining release gates:
```

There is no known target configuration or independent live reference in this
local task. Live ADS, successful UDP Identify and multi-cycle OPC UA/XAE agreement
remain **unverified** until real artifacts are acquired and independently reviewed.
