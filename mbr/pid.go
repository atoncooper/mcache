package mbr

import (
	"math"
	"sync"
)

// PIDController is a standard PID (proportional-integral-derivative) controller
// with integral anti-windup and output clamping.
type PIDController struct {
	Kp, Ki, Kd float64 // coefficients
	setpoint   float64 // target value
	integral   float64 // accumulated integral term
	prevError  float64 // previous error for derivative
	outputMin  float64
	outputMax  float64
	mu         sync.Mutex
}

// PIDConfig holds PID tuning parameters.
type PIDConfig struct {
	Kp       float64 `yaml:"kp"`
	Ki       float64 `yaml:"ki"`
	Kd       float64 `yaml:"kd"`
	Setpoint float64 `yaml:"setpoint"` // e.g. 0.6 for 60% memory target
	Min      float64 `yaml:"min"`      // output lower bound (default -1)
	Max      float64 `yaml:"max"`      // output upper bound (default 1)
}

// DefaultPIDConfig returns sensible defaults for memory-usage control.
func DefaultPIDConfig() PIDConfig {
	return PIDConfig{
		Kp:       1.0,
		Ki:       0.1,
		Kd:       0.05,
		Setpoint: 0.60,
		Min:      -1.0,
		Max:      1.0,
	}
}

// NewPIDController creates a PID controller.
func NewPIDController(cfg PIDConfig) *PIDController {
	return &PIDController{
		Kp:        cfg.Kp,
		Ki:        cfg.Ki,
		Kd:        cfg.Kd,
		setpoint:  cfg.Setpoint,
		outputMin: cfg.Min,
		outputMax: cfg.Max,
	}
}

// Compute calculates the control output given the current measured value and
// the time delta since the last call (seconds). The output is clamped to [min,max].
func (p *PIDController) Compute(measured float64, dt float64) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	if dt <= 0 {
		return 0
	}

	error_ := p.setpoint - measured // positive = below target (need more)

	// Proportional
	proportional := p.Kp * error_

	// Integral with anti-windup
	p.integral += error_ * dt
	integral := p.Ki * p.integral

	// Derivative (on measurement to avoid derivative kick)
	derivative := p.Kd * (error_ - p.prevError) / dt
	p.prevError = error_

	output := proportional + integral + derivative

	// Clamp
	if output > p.outputMax {
		output = p.outputMax
		// Back-calculate integral to prevent windup
		p.integral = (output - proportional - derivative) / p.Ki
		if math.IsInf(p.integral, 0) || math.IsNaN(p.integral) {
			p.integral = 0
		}
	} else if output < p.outputMin {
		output = p.outputMin
		p.integral = (output - proportional - derivative) / p.Ki
		if math.IsInf(p.integral, 0) || math.IsNaN(p.integral) {
			p.integral = 0
		}
	}

	return output
}

// Setpoint returns the target value.
func (p *PIDController) Setpoint() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.setpoint
}

// UpdateSetpoint changes the target value (e.g. when config is reloaded).
func (p *PIDController) UpdateSetpoint(sp float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setpoint = sp
}

// Reset clears the integral and derivative state.
func (p *PIDController) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.integral = 0
	p.prevError = 0
}
