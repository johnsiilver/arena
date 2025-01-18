package arena

import (
	"bytes"
	"context"
	"sync"
	"testing"
)

const (
	KiB = 1024
	MiB = 1048576
)

func BenchmarkStdAllocations(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		allocateMemoryStd()
	}
}

func BenchmarkSyncPoolAllocations(b *testing.B) {
	b.ReportAllocs()

	p := &sync.Pool{
		New: func() any {
			return make([]byte, KiB)
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		allocateMemoryPool(p)
	}
}

func BenchmarkArenaAllocations(b *testing.B) {
	b.ReportAllocs()
	ctx := context.Background()

	pool, err := New(ctx, "testPool", 10*MiB, 1*MiB, 1)
	if err != nil {
		panic(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		allocateMemoryArena(ctx, pool)
	}
}

var sink []byte

func allocateMemoryStd() {
	for x := 0; x < 20*MiB; x += KiB {
		temp := make([]byte, 1024)

		buff := bytes.NewBuffer(temp)
		buff.Write([]byte{byte(x % 256)})
		sink = buff.Bytes()
	}
}

func allocateMemoryPool(p *sync.Pool) {
	for x := 0; x < 20*MiB; x += KiB {
		temp := p.Get().([]byte)

		buff := bytes.NewBuffer(temp)
		buff.Write([]byte{byte(x % 256)})
		p.Put(temp)
	}
}

func allocateMemoryArena(ctx context.Context, p *Pool) {
	for x := 0; x < 20*MiB; x += KiB {
		w, err := p.GetWriter(ctx, KiB)
		if err != nil {
			panic(err)
		}

		_, err = w.Write([]byte{byte(x % 256)})
		if err != nil {
			panic(err)
		}
		w.Release()
	}
}
