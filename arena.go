/*
Package arena provides an arena memory manager for blocks of []byte.

There is an experimental arena pool for Go, but it has been abandoned. That one was an attempt at a more general
purpose arena. That one can be enabled with GOEXPERIMENT=arenas, but it is abandoned.  It is unlikely to be removed
because I believe Google's internal protocol buffer implementation which is several times faster than the open source
version uses it.

This one is specifically for []byte.

This only provides the blocks for []byte. However,
areans are dangerous. You have to be very careful with them as you can unintentionally corrupt memory
or cause allocations.

If you are not a very advanced Go programmer, you should just keep walking past this. If you are that
advanced programmer, you need to have a very good reason to use this. mempool.Pool is usually a better
choice for most use cases. Lots of serialization like in protocol buffers is one of the few good use cases.

You have been warned.

With that said, here is benchmarks on simple usage of the arena (1KiB allocs, a 10MiB arena):

	BenchmarkStdAllocations-10                    93          11082950 ns/op        62914999 B/op      40964 allocs/op
	BenchmarkSyncPoolAllocations-10              141           8547342 ns/op        42451073 B/op      40986 allocs/op
	BenchmarkArenaAllocations-10                 492           2416193 ns/op         1332307 B/op      20482 allocs/op

	This allocates half as many times as either the std allocator or mempool.Pool.
	Vs standard allocation: It uses 47 times less memory than std and is 4 times faster.
	Vs mempool.Pool: It uses 31 times less memory than mempool.Pool and is 3.5 times faster.

So if you are super careful and use this in the exact right place, you can achieve some pretty good results.
*/
package arena

import (
	"context"
	"fmt"
	"sync"
	syncLib "sync"

	"github.com/johnsiilver/arena/internal/mempool"
)

// arenaFullErr is returned when the arena is full.
var arenaFullErr = fmt.Errorf("arena is full")

// arena is a memory manager for blocks of []byte. It is not safe to use this unless you are
// very careful. You can easily corrupt memory or cause allocations. This is only for advanced
// Go programmers.
type arena struct {
	wg         syncLib.WaitGroup
	block      []byte
	reasonable int
	next       int
}

// newArena creates a new Arena with a total size of size. Reasonable is the maximum size of a block request
// from the arena. When a block is requested that is under this size, either the block will come from the arena
// or the arena will be marked as full. If a block is requested that is over this size, a new block will be created
// outside the arena. This is to prevent the arena from being used for large allocations. The reasonable size
// must be at less than 1/10th the size of the arena.
func newArena(size, reasonable uint) (*arena, error) {
	if reasonable*10 > size {
		return nil, fmt.Errorf("reasonable block size is too large for arena")
	}
	return &arena{
		block:      make([]byte, size),
		reasonable: int(reasonable),
	}, nil
}

// Get returns a block of bytes from the arena. You must be careful not to change the slice size which
// will lead to memory corruption. This is thread safe.
func (a *arena) GetWriter(size int) (Writer, error) {
	if size < 1 {
		return Writer{}, fmt.Errorf("size must be greater than 0")
	}

	if size > a.reasonable {
		return Writer{
			block: make([]byte, size),
			// There is no passing of wg, because this is not allocated on the arena.
		}, nil
	}

	if a.next+size > len(a.block) {
		return Writer{}, arenaFullErr
	}

	a.wg.Add(1)

	w := Writer{
		block: a.block[a.next : a.next+size],
		wg:    &a.wg,
	}

	a.next += size
	return w, nil // ignore copylocks
}

// Wait waits for all memory allocations to complete.
func (a *arena) Wait() {
	a.wg.Wait()
}

// Reset resets the arena to be reused. We do not zero out the memory.
func (a *arena) Reset(ctx context.Context) {
	a.next = 0
}

// Pool manages a pool of Arenas.
type Pool struct {
	size, reasonable uint

	mu      sync.Mutex
	current *arena
	pool    mempool.Pool[*arena]
	arenas  []*arena
}

func New(ctx context.Context, name string, size, reasonable uint, buffer int) (*Pool, error) {
	if reasonable*10 > size {
		return nil, fmt.Errorf("reasonable block size is too large for arena")
	}
	p := &Pool{
		size:       size,
		reasonable: reasonable,
		pool: mempool.NewPool[*arena](
			ctx,
			name,
			func() *arena {
				a, err := newArena(size, reasonable)
				if err != nil {
					panic(err)
				}
				return a
			},
			mempool.WithBuffer(buffer),
		),
	}
	a, err := newArena(size, reasonable)
	if err != nil {
		return nil, err
	}
	p.current = a
	return p, nil
}

func (p *Pool) GetWriter(ctx context.Context, size int) (Writer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.getWriter(ctx, size)
}

func (p *Pool) getWriter(ctx context.Context, size int) (Writer, error) {
	w, err := p.current.GetWriter(size)
	if err == arenaFullErr {
		p.handleDepleted(ctx, p.current)

		a := p.pool.Get(ctx)
		p.current = a
		return a.GetWriter(size)
	}
	return w, err // ignore copylocks
}

// handleDepleted spins off a goroutine and waits for the waitgroup on the arena to complete.
// Once it is complete, it resets the arena and puts it back in the next channel. If that channel is
// full, it will just drop the arena.
func (p *Pool) handleDepleted(ctx context.Context, a *arena) {
	go func() {
		a.Wait()
		p.pool.Put(ctx, a)
	}()
}
