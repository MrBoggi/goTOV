package main

import (
	"strings"
	"testing"

	"github.com/siyka-au/go-ads/v3/ams"
)

func TestClassifyErrorSeparatesAMSRouterAndPLCRuntime(t *testing.T) {
	router := classifyError(ams.RouterError{Code: ams.ReturnCodeGlobalTargetPortNotFound})
	if !strings.Contains(router, "AMS router rejection") || !strings.Contains(router, "0x00000006") {
		t.Fatalf("router error classification = %q", router)
	}

	runtime := classifyError(ams.ReturnCodeDeviceAccessDenied)
	if !strings.Contains(runtime, "PLC runtime ADS response") || !strings.Contains(runtime, "0x00000723") {
		t.Fatalf("runtime error classification = %q", runtime)
	}
}
