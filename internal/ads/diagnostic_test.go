package ads

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"testing"
	"time"

	"github.com/MrBoggi/goTOV/internal/config"
	"github.com/stretchr/testify/require"
)

type sampleReader struct {
	values []Inputs
	err    error
	calls  int
}

func (r *sampleReader) ReadInputsSnapshot(context.Context) (Inputs, error) {
	r.calls++
	if r.err != nil {
		return Inputs{}, r.err
	}
	i := r.calls - 1
	if i >= len(r.values) {
		i = len(r.values) - 1
	}
	return r.values[i], nil
}

func contractInput(t *testing.T, sequence uint32) Inputs {
	t.Helper()
	b, err := hex.DecodeString(validV1Vector)
	require.NoError(t, err)
	binary.LittleEndian.PutUint32(b[4:], sequence)
	input, err := decodeStableInputs(b, b, b)
	require.NoError(t, err)
	return input
}

func TestCollectDiagnosticCycles(t *testing.T) {
	reader := &sampleReader{values: []Inputs{contractInput(t, 2), contractInput(t, 2), contractInput(t, 4), contractInput(t, 6)}}
	samples, err := CollectDiagnostic(context.Background(), reader, 3, time.Nanosecond)
	require.NoError(t, err)
	require.Len(t, samples, 3)
	require.Equal(t, 4, reader.calls)
	require.Equal(t, uint32(6), samples[2].Inputs.Sequence)
	for _, sample := range samples {
		require.False(t, sample.FinishedAt.Before(sample.StartedAt))
	}
	reader = &sampleReader{values: []Inputs{contractInput(t, 2)}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	samples, err = CollectDiagnostic(ctx, reader, 3, time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Len(t, samples, 1)
	_, err = CollectDiagnostic(context.Background(), reader, 2, time.Second)
	require.Error(t, err)
	reader = &sampleReader{values: []Inputs{contractInput(t, 4), contractInput(t, 2)}}
	_, err = CollectDiagnostic(context.Background(), reader, 3, time.Nanosecond)
	require.ErrorContains(t, err, "regressed")
	reader = &sampleReader{err: fmt.Errorf("read failed")}
	_, err = CollectDiagnostic(context.Background(), reader, 3, time.Nanosecond)
	require.ErrorContains(t, err, "read failed")
	reader = &sampleReader{values: []Inputs{contractInput(t, 0xfffffffe), contractInput(t, 0), contractInput(t, 2)}}
	_, err = CollectDiagnostic(context.Background(), reader, 3, time.Nanosecond)
	require.NoError(t, err)
}

func evidence(t *testing.T) DiagnosticReport {
	t.Helper()
	report := DiagnosticReport{Source: "ads", Target: "test-runtime:851", Symbol: SymbolMetadata{Name: InputsSymbol, DataType: "ST_BreweryInputs", Size: 52}}
	at := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		started := at.Add(time.Duration(i) * time.Second)
		report.Samples = append(report.Samples, DiagnosticSample{StartedAt: started, FinishedAt: started.Add(time.Millisecond), Inputs: contractInput(t, uint32(2+i*2))})
	}
	return report
}

func TestCompareDiagnosticContractAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*DiagnosticReport)
	}{
		{"matches", func(*DiagnosticReport) {}},
		{"signed zero mismatch", func(r *DiagnosticReport) { r.Samples[0].Inputs.FlowHLT = math.Float32frombits(0x80000000) }},
		{"mismatch", func(r *DiagnosticReport) { r.Samples[1].Inputs.FlowWort++ }},
		{"flag mismatch", func(r *DiagnosticReport) { r.Samples[0].Inputs.FlowHLTOutOfRange = false }},
		{"integer mismatch", func(r *DiagnosticReport) { r.Samples[2].Inputs.GlycolChillerTemp++ }},
		{"wrong source", func(r *DiagnosticReport) { r.Source = "ads" }},
		{"wrong target", func(r *DiagnosticReport) { r.Target = "other" }},
		{"partial", func(r *DiagnosticReport) { r.Error = "read failed" }},
		{"short", func(r *DiagnosticReport) { r.Samples = r.Samples[:2] }},
		{"duplicate cycle", func(r *DiagnosticReport) { r.Samples[1].Inputs.Sequence = 2 }},
		{"regression", func(r *DiagnosticReport) { r.Samples[2].Inputs.Sequence = 0 }},
		{"odd", func(r *DiagnosticReport) { r.Samples[0].Inputs.Sequence = 3 }},
		{"wrong version", func(r *DiagnosticReport) { r.Samples[0].Inputs.Version = 2 }},
		{"wrong layout", func(r *DiagnosticReport) { r.Samples[0].Inputs.StructureSize = 51 }},
		{"wrong symbol", func(r *DiagnosticReport) { r.Symbol.Name = "MAIN.fbUA" }},
		{"wrong size", func(r *DiagnosticReport) { r.Symbol.Size = 51 }},
		{"missing time", func(r *DiagnosticReport) { r.Samples[0].StartedAt = time.Time{} }},
		{"reversed time", func(r *DiagnosticReport) { r.Samples[0].FinishedAt = r.Samples[0].StartedAt.Add(-time.Second) }},
		{"overlong window", func(r *DiagnosticReport) { r.Samples[2].FinishedAt = r.Samples[2].StartedAt.Add(2 * time.Second) }},
		{"stale reference", func(r *DiagnosticReport) {
			for i := range r.Samples {
				r.Samples[i].StartedAt = r.Samples[i].StartedAt.Add(5 * time.Second)
				r.Samples[i].FinishedAt = r.Samples[i].FinishedAt.Add(5 * time.Second)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture, reference := evidence(t), evidence(t)
			reference.Source = "xae"
			reference.ReferenceEvidence = "synthetic fixture, same publisher cycles"
			if tc.name == "signed zero mismatch" {
				capture.Samples[0].Inputs.FlowHLT = 0
				reference.Samples[0].Inputs.FlowHLT = 0
			}
			tc.mutate(&reference)
			err := CompareDiagnostics(capture, reference, time.Second)
			if tc.name == "matches" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestReadDiagnosticRejectsMissingFalseOrZeroFields(t *testing.T) {
	report := evidence(t)
	b, err := json.Marshal(report)
	require.NoError(t, err)
	decoded, err := ReadDiagnostic(bytes.NewReader(b))
	require.NoError(t, err)
	require.Equal(t, report, decoded)
	for _, input := range []string{
		`{"Samples":[{"Inputs":{}}]}`,
		string(bytes.Replace(b, []byte(`"FlowMLTOutOfRange":false,`), nil, 1)),
		string(bytes.Replace(b, []byte(`"FlowMLTOutOfRange":false`), []byte(`"FlowMLTOutOfRange":null`), 1)),
	} {
		_, err = ReadDiagnostic(bytes.NewBufferString(input))
		require.Error(t, err)
	}
}

// Exercise the complete diagnostic loop with the real SDK and the existing
// strict transport allowlist. No app or control engine is involved.
func TestDiagnosticTransport(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, listener.Close()) }()
	fake := &fakeADS{size: 52, handle: fakeInputHandle, advanceCycles: true}
	done := make(chan error, 1)
	go func() { done <- fake.serve(listener) }()
	client, err := NewClient(config.ADSConfig{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, TargetNetID: "1.2.3.4.1.1", LocalNetID: "1.2.3.5.1.1", TargetPort: 851, LocalPort: 10500, RequestTimeout: "1s", PollInterval: "1s"})
	require.NoError(t, err)
	defer func() { require.NoError(t, client.Close()) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.Connect(ctx))
	require.NoError(t, client.ReadState(ctx))
	require.NoError(t, client.LookupInputs(ctx))
	require.Equal(t, SymbolMetadata{Name: InputsSymbol, Size: 52, DataType: "ST_BreweryInputs"}, client.InputsMetadata())
	samples, err := CollectDiagnostic(ctx, client, 3, time.Nanosecond)
	require.NoError(t, err)
	require.Len(t, samples, 3)
	require.Equal(t, uint32(6), samples[2].Inputs.Sequence)
	require.NoError(t, client.Close())
	require.NoError(t, <-done)
	require.Equal(t, 9, fake.reads)
	require.Equal(t, "release", fake.calls[len(fake.calls)-1])
}
