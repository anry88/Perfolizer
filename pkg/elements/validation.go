package elements

import (
	"fmt"
	"math"
	"time"
)

func ValidateUsers(value int) error {
	if value < 1 {
		return fmt.Errorf("Users must be greater than or equal to 1")
	}
	return nil
}

func ValidateIterations(value int) error {
	if value < -1 {
		return fmt.Errorf("Iterations must be greater than or equal to -1")
	}
	return nil
}

func ValidateRPS(field string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s must be a finite number", field)
	}
	if value < 0 {
		return fmt.Errorf("%s must be greater than or equal to 0", field)
	}
	return nil
}

func ValidateDuration(field string, value time.Duration) error {
	if value < 0 {
		return fmt.Errorf("%s must be greater than or equal to 0 ms", field)
	}
	return nil
}
