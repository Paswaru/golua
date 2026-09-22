package base_test

import (
	"io"
	"sync"
	"testing"

	"github.com/arnodel/golua/lib"
	rt "github.com/arnodel/golua/runtime"
)

// TestLoadDoesNotRaceWithRunningRuntime checks that loading the base library
// into a Runtime writes no state that another, already running Runtime reads.
// Embedders that build a Runtime per connection or per request do exactly this.
//
// ipairsIterator and nextGoFunc are shared by every Runtime in the process, and
// GoCont.RunInThread reads their compliance flags on every ipairs/pairs/next
// call, so Load must not write those flags.
//
// This test only detects the problem under -race.
func TestLoadDoesNotRaceWithRunningRuntime(t *testing.T) {
	const src = `
		local t = {1, 2, 3}
		for i = 1, 1000 do
			for _, _ in ipairs(t) do end
			for _, _ in pairs(t) do end
			next(t)
		end
	`
	r := rt.New(io.Discard)
	lib.LoadAll(r)
	clos, err := r.CompileAndLoadLuaChunk("racetest", []byte(src), rt.TableValue(r.GlobalEnv()))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := rt.Call1(r.MainThread(), rt.FunctionValue(clos)); err != nil {
			t.Error(err)
		}
	}()
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r2 := rt.New(io.Discard)
			lib.LoadAll(r2)
			r2.Close(nil)
		}()
	}
	wg.Wait()
	r.Close(nil)
}
