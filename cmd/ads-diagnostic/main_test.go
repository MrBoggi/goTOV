package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiagnosticRequiresExplicitConfiguration(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--live"}, {"--capture-file", "unused"},
		{"--live", "--capture-file", "unused", "--reference-file", "unused"},
		{"--live", "--port", "48898", "--target-port", "65536", "--local-port", "10500"},
		{"unexpected"},
	} {
		var out bytes.Buffer
		require.Error(t, run(args, &out))
		require.Empty(t, out.String())
	}
}

func TestOfflineComparisonCommand(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, run([]string{"--capture-file", "testdata/ads.json", "--reference-file", "testdata/xae.json"}, &out))
	require.Contains(t, out.String(), `"comparison":"matched"`)
	require.Contains(t, out.String(), `"cycles":3`)
	out.Reset()
	require.Error(t, run([]string{"--capture-file", "testdata/ads.json", "--reference-file", "testdata/ads.json"}, &out))
	require.Empty(t, out.String())
}
