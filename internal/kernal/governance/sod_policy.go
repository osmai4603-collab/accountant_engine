package governance

import "errors"

var ErrSegregationOfDuties = errors.New("segregation of duties violation")

func ValidateSegregationOfDuties(actorA, actorB string, dutyA, dutyB string) error {
	if actorA != "" && actorA == actorB && dutyA != dutyB {
		return ErrSegregationOfDuties
	}
	return nil
}
