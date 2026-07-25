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

type PIDConfig struct {
	P float64 `json:"p"`
	I float64 `json:"i"`
	D float64 `json:"d"`
}

type AnalogActuator struct {
	Mode     ActuatorMode `json:"mode"`
	Setpoint float64      `json:"setpoint"`
	Command  float64      `json:"command"` // Calculated/written command (0-100)
	State    float64      `json:"state"`   // Actual from sensor
	IsPID    bool         `json:"isPid"`
	PID      PIDConfig    `json:"pid"`
}

type BrewhouseState struct {
	Valves        map[string]*DigitalActuator `json:"valves"`
	Pumps         map[string]*DigitalActuator `json:"pumps"`
	BKHeater      *AnalogActuator             `json:"bkHeater"`
	MLTHeater     *AnalogActuator             `json:"mltHeater"`
	ProportionalV *AnalogActuator             `json:"proportionalValve"`
	Sensors       map[string]float64          `json:"sensors"`
}

func InitialState() *BrewhouseState {
	return &BrewhouseState{
		Valves: map[string]*DigitalActuator{
			"V1_VannInn":       {Mode: ModeManual},
			"V2_HLT_TilMLT":    {Mode: ModeManual},
			"V3_MLT_TilBK":     {Mode: ModeManual},
			"V4_BK_TilHX":      {Mode: ModeManual},
			"V5_HX_TilDrain":   {Mode: ModeManual},
			"V6_HX_TilFerment": {Mode: ModeManual},
			"V7_SpargeWater":   {Mode: ModeManual},
			"V8_ChillerIn":     {Mode: ModeManual},
			"V9_ChillerOut":    {Mode: ModeManual},
		},
		Pumps: map[string]*DigitalActuator{
			"pumpeHLT":  {Mode: ModeManual},
			"pumpeWort": {Mode: ModeManual},
		},
		BKHeater:      &AnalogActuator{Mode: ModeManual},
		MLTHeater:     &AnalogActuator{Mode: ModeManual},
		ProportionalV: &AnalogActuator{Mode: ModeManual},
		Sensors:       make(map[string]float64),
	}
}
