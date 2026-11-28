package posting

import "errors"

var (
	ErrZeroAmountLine = errors.New("zero amount on accounting line")
	ErrUnbalancedMove = errors.New("unbalanced accounting move")
)
