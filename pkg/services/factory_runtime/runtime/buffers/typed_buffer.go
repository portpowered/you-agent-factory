package buffers

import "context"

// TypedBuffer is a generic, non-blocking, channel-based buffer for passing
// typed messages between factory components.
type TypedBuffer[T any] struct {
	ch     chan T
	onDrop func()
}

func NewTypedBuffer[T any](capacity int) *TypedBuffer[T] {
	if capacity <= 0 {
		capacity = 64
	}
	return &TypedBuffer[T]{ch: make(chan T, capacity)}
}

func (b *TypedBuffer[T]) SetOnDrop(fn func()) {
	b.onDrop = fn
}

func (b *TypedBuffer[T]) Write(ctx context.Context, data T) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	select {
	case b.ch <- data:
		return true
	case <-ctx.Done():
		return false
	default:
		if b.onDrop != nil {
			b.onDrop()
		}
		return false
	}
}

func (b *TypedBuffer[T]) Read() (T, bool) {
	select {
	case data := <-b.ch:
		return data, true
	default:
		var zero T
		return zero, false
	}
}

func (b *TypedBuffer[T]) HasData() bool {
	return len(b.ch) > 0
}

func (b *TypedBuffer[T]) Len() int {
	return len(b.ch)
}

func (b *TypedBuffer[T]) Cap() int {
	return cap(b.ch)
}

func (b *TypedBuffer[T]) Chan() <-chan T {
	return b.ch
}
