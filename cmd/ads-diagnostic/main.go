// ads-diagnostic is a standalone evidence tool. It never imports app, starts a
// control engine, loads application configuration or reads credential files.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/MrBoggi/goTOV/internal/ads"
	"github.com/MrBoggi/goTOV/internal/config"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("ads-diagnostic", flag.ContinueOnError)
	live := flags.Bool("live", false, "explicitly enable PLC reads using an existing route")
	host := flags.String("host", "", "known ADS host; required for live reads")
	target := flags.String("target-net-id", "", "known target NetID")
	local := flags.String("local-net-id", "", "existing routed local NetID")
	port := flags.Int("port", 0, "known ADS TCP port")
	targetPort := flags.Uint("target-port", 0, "known runtime AMS port")
	localPort := flags.Uint("local-port", 0, "known local AMS port")
	count := flags.Int("cycles", 5, "number of distinct cycles (minimum 3)")
	interval := flags.Duration("interval", time.Second, "delay between read attempts")
	timeout := flags.Duration("timeout", time.Minute, "total live diagnostic deadline")
	captureFile := flags.String("capture-file", "", "offline ADS evidence JSON")
	referenceFile := flags.String("reference-file", "", "offline independent XAE/OPC UA evidence JSON")
	maxSkew := flags.Duration("max-skew", time.Second, "maximum evidence window and cross-source start skew")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *captureFile != "" || *referenceFile != "" {
		if *live || *captureFile == "" || *referenceFile == "" {
			return fmt.Errorf("offline comparison requires both files and no --live")
		}
		capture, err := readReport(*captureFile)
		if err != nil {
			return err
		}
		reference, err := readReport(*referenceFile)
		if err != nil {
			return err
		}
		if err = ads.CompareDiagnostics(capture, reference, *maxSkew); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(map[string]any{"comparison": "matched", "cycles": len(capture.Samples), "target": capture.Target, "reference_source": reference.Source})
	}
	if !*live {
		return fmt.Errorf("no network access: specify --live with explicit target configuration, or both offline files")
	}
	if *port < 1 || *port > 65535 || *targetPort < 1 || *targetPort > 65535 || *localPort < 1 || *localPort > 65535 || *timeout <= 0 || *interval <= 0 || *count < 3 || *count > 10000 {
		return fmt.Errorf("invalid ports, timeout, interval or cycle count")
	}
	cfg := config.ADSConfig{Host: *host, Port: *port, TargetNetID: *target, LocalNetID: *local, TargetPort: uint16(*targetPort), LocalPort: uint16(*localPort), RequestTimeout: "8s", PollInterval: interval.String()}
	client, err := ads.NewClient(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report := ads.DiagnosticReport{Source: "ads", Target: fmt.Sprintf("%s:%d", *target, *targetPort)}
	err = client.Connect(ctx)
	if err == nil {
		err = client.ReadState(ctx)
	}
	if err == nil {
		err = client.LookupInputs(ctx)
	}
	report.Symbol = client.InputsMetadata()
	if err == nil {
		report.Samples, err = ads.CollectDiagnostic(ctx, client, *count, *interval)
	}
	err = errors.Join(err, client.Close())
	if err != nil {
		report.Error = err.Error()
	}
	return errors.Join(err, json.NewEncoder(out).Encode(report))
}

func readReport(path string) (ads.DiagnosticReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return ads.DiagnosticReport{}, err
	}
	report, readErr := ads.ReadDiagnostic(f)
	return report, errors.Join(readErr, f.Close())
}
