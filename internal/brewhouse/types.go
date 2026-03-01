package brewhouse

type ActuatorMode string

const (
	ModeAuto   ActuatorMode = "Auto"
	ModeManual ActuatorMode = "Manual"
)

type DigitalActuator struct {
	Mode    ActuatorMode `json:"mode"`
	Command bool         `json:"command"`
	State   bool         `json:"state"` // Actual state from OPC UA
}

type AnalogActuator struct {
	Mode     ActuatorMode `json:"mode"`
	Setpoint float64      `json:"setpoint"`
	Command  float64      `json:"command"` // Calculated/written command (0-100)
	State    float64      `json:"state"`   // Actual from sensor
}

type BrewhouseState struct {
	Valves        map[string]*DigitalActuator `json:"valves"`
	Pumps         map[string]*DigitalActuator `json:"pumps"`
	Heater        *AnalogActuator             `json:"heater"`
	ProportionalV *AnalogActuator             `json:"proportionalValve"`
	Sensors       map[string]float64          `json:"sensors"`
}

func InitialState() *BrewhouseState {
	return &BrewhouseState{
		Valves: map[string]*DigitalActuator{
			"V1": {Mode: ModeManual},
			"V2": {Mode: ModeManual},
			"V3": {Mode: ModeManual},
			"V4": {Mode: ModeManual},
			"V5": {Mode: ModeManual},
			"V6": {Mode: ModeManual},
			"V7": {Mode: ModeManual},
			"V8": {Mode: ModeManual},
			"V9": {Mode: ModeManual},
		},
		Pumps: map[string]*DigitalActuator{
			"Pump1": {Mode: ModeManual},
			"Pump2": {Mode: ModeManual},
		},
		Heater:        &AnalogActuator{Mode: ModeManual},
		ProportionalV: &AnalogActuator{Mode: ModeManual},
		Sensors:       make(map[string]float64),
	}
}
