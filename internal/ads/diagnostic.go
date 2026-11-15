package ads

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"time"
)

// DiagnosticReport is a local evidence artifact, not a production API contract.
// Source is ads, xae or opcua; Target identifies the same PLC/runtime in both files.
type DiagnosticReport struct {
	Source            string
	ReferenceEvidence string // Reference-only: independent capture artifact and cycle-correlation method.
	Target            string
	Symbol            SymbolMetadata
	Samples           []DiagnosticSample
	Error             string
}

type SymbolMetadata struct {
	Name     string
	DataType string
	Size     uint32
}

type DiagnosticSample struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Inputs     Inputs
}

type diagnosticReader interface {
	ReadInputsSnapshot(context.Context) (Inputs, error)
}

// CollectDiagnostic records only validated snapshots from distinct forward cycles.
// Repeated sequence values do not count as additional PLC cycles. Errors fail
// closed; callers retain the partial evidence and must not mark it successful.
func CollectDiagnostic(ctx context.Context, reader diagnosticReader, count int, interval time.Duration) ([]DiagnosticSample, error) {
	if count < 3 || count > 10000 || interval <= 0 {
		return nil, fmt.Errorf("diagnostics require 3..10000 distinct cycles and a positive interval")
	}
	samples := make([]DiagnosticSample, 0, count)
	for len(samples) < count {
		started := time.Now().UTC()
		inputs, err := reader.ReadInputsSnapshot(ctx)
		if err != nil {
			return samples, err
		}
		if inputs.Version != InputsVersion || inputs.StructureSize != InputsSize || inputs.Sequence%2 != 0 {
			return samples, fmt.Errorf("invalid diagnostic snapshot contract")
		}
		if len(samples) == 0 || inputs.Sequence != samples[len(samples)-1].Inputs.Sequence {
			if len(samples) > 0 && !sequenceAfter(inputs.Sequence, samples[len(samples)-1].Inputs.Sequence) {
				return samples, fmt.Errorf("sequence regressed or PLC restarted")
			}
			samples = append(samples, DiagnosticSample{StartedAt: started, FinishedAt: time.Now().UTC(), Inputs: inputs})
		}
		if len(samples) == count {
			break
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return samples, ctx.Err()
		case <-timer.C:
		}
	}
	return samples, nil
}

func sequenceAfter(next, previous uint32) bool {
	delta := next - previous
	return delta > 0 && delta < 1<<31
}

// ReadDiagnostic rejects incomplete input objects so absent zero/false fields
// cannot accidentally become successful reference evidence.
func ReadDiagnostic(r io.Reader) (DiagnosticReport, error) {
	data, err := io.ReadAll(io.LimitReader(r, 16*1024*1024+1))
	if err != nil {
		return DiagnosticReport{}, err
	}
	if len(data) > 16*1024*1024 {
		return DiagnosticReport{}, fmt.Errorf("diagnostic file exceeds 16 MiB")
	}
	var report DiagnosticReport
	if err = json.Unmarshal(data, &report); err != nil {
		return report, err
	}
	var raw struct {
		Samples []struct{ Inputs map[string]json.RawMessage }
	}
	if err = json.Unmarshal(data, &raw); err != nil {
		return report, err
	}
	typ := reflect.TypeOf(Inputs{})
	for i, sample := range raw.Samples {
		if len(sample.Inputs) != typ.NumField() {
			return report, fmt.Errorf("sample %d requires every Inputs field", i)
		}
		for j := 0; j < typ.NumField(); j++ {
			value, ok := sample.Inputs[typ.Field(j).Name]
			if !ok || string(value) == "null" {
				return report, fmt.Errorf("sample %d missing %s", i, typ.Field(j).Name)
			}
		}
	}
	return report, nil
}

// CompareDiagnostics compares all raw contract fields exactly. References must
// be independently observed OPC UA/XAE values, correlated with the stable ADS
// publisher sequence. Time proximity alone does not establish a shared cycle.
// Float32 equality is intentional: no unapproved scaling or tolerance is inferred.
func CompareDiagnostics(capture, reference DiagnosticReport, maxSkew time.Duration) error {
	if capture.Source != "ads" || (reference.Source != "xae" && reference.Source != "opcua") {
		return fmt.Errorf("require ads capture and independent xae/opcua reference")
	}
	if reference.ReferenceEvidence == "" {
		return fmt.Errorf("independent reference provenance and cycle correlation are required")
	}
	if capture.Target == "" || capture.Target != reference.Target {
		return fmt.Errorf("target identity missing or mismatched")
	}
	if maxSkew <= 0 {
		return fmt.Errorf("max skew must be positive")
	}
	for _, report := range []DiagnosticReport{capture, reference} {
		if report.Error != "" {
			return fmt.Errorf("incomplete diagnostic: %s", report.Error)
		}
		if report.Symbol.Name != InputsSymbol || report.Symbol.Size != InputsSize {
			return fmt.Errorf("invalid symbol metadata")
		}
		if len(report.Samples) < 3 {
			return fmt.Errorf("require at least three distinct cycles")
		}
		for i, sample := range report.Samples {
			if sample.StartedAt.IsZero() || sample.FinishedAt.Before(sample.StartedAt) {
				return fmt.Errorf("invalid sample time window")
			}
			if sample.FinishedAt.Sub(sample.StartedAt) > maxSkew {
				return fmt.Errorf("sample time window exceeds max skew")
			}
			if sample.Inputs.Version != InputsVersion || sample.Inputs.StructureSize != InputsSize || sample.Inputs.Sequence%2 != 0 {
				return fmt.Errorf("invalid sample contract")
			}
			if i > 0 && (!sequenceAfter(sample.Inputs.Sequence, report.Samples[i-1].Inputs.Sequence) || sample.StartedAt.Before(report.Samples[i-1].FinishedAt)) {
				return fmt.Errorf("cycles must advance with ordered time windows")
			}
		}
	}
	if len(capture.Samples) != len(reference.Samples) {
		return fmt.Errorf("sample counts differ")
	}
	for i, sample := range capture.Samples {
		other := reference.Samples[i]
		skew := sample.StartedAt.Sub(other.StartedAt)
		if skew < 0 {
			skew = -skew
		}
		if skew > maxSkew {
			return fmt.Errorf("sample %d reference exceeds max skew", i)
		}
		if sample.Inputs != other.Inputs {
			return fmt.Errorf("sample %d sequence %d: input values or sequence differ", i, sample.Inputs.Sequence)
		}
	}
	return nil
}
