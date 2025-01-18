package mempool

import (
	"context"
	"sync"
)

// Resetter is an interface that can be implemented by a type to allow its values to be reset.
type Resetter interface {
	Reset(ctx context.Context)
}

// Pool is an advanced generics based sync.Pool. The generics make for less verbose
// code and prevent accidental assignments of the wrong type which can cause a panic.
// This is NOT a drop in replacement for sync.Pool as we need to provide methods
// a context object.
//
// In addition it can provide:
// - If the type T implements the Resetter interface, the Reset() method will be called on the value before it is returned to the pool.
type Pool[T any] struct {
	p sync.Pool

	buffer chan T

	mu sync.Mutex // For CopyLocks error
}

type opts struct {
	bufferSize int
}

// Option is an option for constructors in this package.
type Option func(opts) (opts, error)

// WithBuffer sets the buffer for the pool.
// This is a channel of available values that are not in the pool. If this is set to 10 a
// channel of capacity of 10 will be created. The Pool
// will always use the buffer before creating new values. It will always attempt to put
// values back into the buffer before putting them in the pool. If not set, the pool
// will only use the sync.Pool.
func WithBuffer(size int) Option {
	return func(o opts) (opts, error) {
		o.bufferSize = size
		return o, nil
	}
}

// NewPool creates a new Pool for use. This returns a non-pointer value. If passing Pool, you must use a
// reference to it like a normal sync.Pool (aka use &pool not pool). "name" is used to create a
// new meter with the name:
//
//	"[package path]/[package name]:sync.Pool([type stored])/[name]".
func NewPool[T any](ctx context.Context, name string, n func() T, options ...Option) Pool[T] {
	opts := opts{}
	var err error
	for _, o := range options {
		opts, err = o(opts)
		if err != nil {
			panic(err)
		}
	}

	var c chan T
	if opts.bufferSize > 0 {
		c = make(chan T, opts.bufferSize)
	}

	p := Pool[T]{
		buffer: c,
	}

	p.p = sync.Pool{
		New: func() any {
			return n()
		},
	}
	// Note; CopyLocks warning is fine here, because we haven't used the pool yet. It is safe to copy.
	// We want to treat it as normal sync.Pool. The user can decide if they want to use a reference.
	// We put a mutex in our struct to cause a CopyLocks warning.
	return p
}

// Get returns a value from the pool or creates a new one if the pool is empty.
func (p *Pool[T]) Get(ctx context.Context) T {
	select {
	case v := <-p.buffer:
		return v
	default:
	}

	return p.p.Get().(T)
}

// Put puts a value back into the pool.
func (p *Pool[T]) Put(ctx context.Context, v T) {
	if r, ok := any(v).(Resetter); ok {
		r.Reset(ctx)
	}

	select {
	case p.buffer <- v:
		return
	default:
	}
	p.p.Put(v)
}
