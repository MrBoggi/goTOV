package fermentation

import "time"

const (
	Tank1 = 0
	Tank2 = 1
)

type TankInput struct {
	Active     bool
	Temp       float64
	TargetTemp float64
}

type TankOutput struct {
	Heating bool
	Cooling bool
}

type EngineOutputs struct {
	Tanks [2]TankOutput
	Pump  bool
}

type EngineState struct {
	Outputs EngineOutputs
}

func NewEngineState() EngineState {
	return EngineState{Outputs: EngineOutputs{}}
}

type EngineConfig struct {
	CoolingHysteresis float64       // degrees above target
	HeatingHysteresis float64       // degrees below target
	PumpDelay         time.Duration // delay before enabling pump after cooling starts
}

func (s EngineState) Tick(
	cfg EngineConfig,
	now time.Time,
	t1 TankInput,
	t2 TankInput,
) EngineState {

	out := EngineOutputs{}

	// TANK 1
	if t1.Active {
		if t1.Temp > t1.TargetTemp+cfg.CoolingHysteresis {
			out.Tanks[Tank1].Cooling = true
		}
		if t1.Temp < t1.TargetTemp-cfg.HeatingHysteresis {
			out.Tanks[Tank1].Heating = true
		}
	}

	// TANK 2
	if t2.Active {
		if t2.Temp > t2.TargetTemp+cfg.CoolingHysteresis {
			out.Tanks[Tank2].Cooling = true
		}
		if t2.Temp < t2.TargetTemp-cfg.HeatingHysteresis {
			out.Tanks[Tank2].Heating = true
		}
	}

	// Shared pump runs if any tank is cooling
	out.Pump = out.Tanks[Tank1].Cooling || out.Tanks[Tank2].Cooling

	s.Outputs = out
	return s
}
