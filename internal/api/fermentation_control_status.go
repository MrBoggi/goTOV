package api

import (
	"encoding/json"
	"net/http"
)

// fermentationControlStatusV1 describes this application's present wiring.
// The dedicated supervisor is not attached to the server. Neither this status
// nor the UA control-path selection is an observation of the PLC's owner.
type fermentationControlStatusV1 struct {
	ContractVersion       int     `json:"contract_version"`
	ConfiguredControlPath string  `json:"configured_control_path"`
	ADSSupervisor         string  `json:"ads_supervisor"`
	ReportedOwner         *string `json:"reported_owner"`
	WatchdogHealthy       *bool   `json:"watchdog_healthy"`
	State                 string  `json:"state"`
	Reason                string  `json:"reason"`
	ADSSelectionAvailable bool    `json:"ads_selection_available"`
}

func (s *Server) handleFermentationControlStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(fermentationControlStatusV1{ContractVersion: 1, ConfiguredControlPath: "ua", ADSSupervisor: "not_started", State: "degraded", Reason: "ownership_unverified"})
}
