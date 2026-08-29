package headless

import "errors"

var ErrBusy = errors.New("headless lock busy")
