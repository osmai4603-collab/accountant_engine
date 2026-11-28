package journal

import "errors"

var (
	ErrMoveAlreadyPosted      = errors.New("accounting move already posted")
	ErrCannotModifyPostedMove = errors.New("cannot modify posted move")
)

