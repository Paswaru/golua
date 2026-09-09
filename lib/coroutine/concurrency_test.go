package coroutine_test

import (
	"io"
	"testing"

	"github.com/arnodel/golua/lib"
	rt "github.com/arnodel/golua/runtime"
)

// TestCoroutineEndDoesNotRaceContextManager checks that a coroutine finishing
// does not touch the Runtime's context manager after it has handed control back
// to the thread that resumed it.
//
// Thread.end runs on the coroutine's own goroutine. sendResumeValues wakes the
// resuming goroutine, so anything the ending thread does afterwards runs
// alongside it, and the two threads share one runtimeContextManager: pcall
// pushes a context on the resumer while the ending coroutine releases its
// goroutine stack.
//
// This test only detects the problem under -race.
func TestCoroutineEndDoesNotRaceContextManager(t *testing.T) {
	const src = `
		for i = 1, 500 do
			local co = coroutine.wrap(function() coroutine.yield(1) return 2 end)
			co()
			co()
			pcall(function() return i end)
		end
	`
	r := rt.New(io.Discard)
	lib.LoadAll(r)
	clos, err := r.CompileAndLoadLuaChunk("coroutinerace", []byte(src), rt.TableValue(r.GlobalEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Call1(r.MainThread(), rt.FunctionValue(clos)); err != nil {
		t.Fatal(err)
	}
	r.Close(nil)
}
