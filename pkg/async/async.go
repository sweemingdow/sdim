package async

import "context"

type Task func()

type AsyncHandler interface {
	Submit(task Task) error

	Shutdown(ctx context.Context) error
}
