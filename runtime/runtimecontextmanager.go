//go:build !noquotas
// +build !noquotas

package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/arnodel/golua/runtime/internal/luagc"
)

const QuotasAvailable = true

// When tracking time, the runtimeContextManager will take the opportunity to
// update the time spent in the context when the CPU is increased.  It doesn't
// do that every time because it would slow down the runtime too much for little
// benefit.  This value sets the amount of CPU required that triggers a time
// update.
const cpuThresholdIncrement = 10000

type runtimeContextManager struct {
	hardLimits    RuntimeResources
	softLimits    RuntimeResources
	usedResources RuntimeResources

	requiredFlags ComplianceFlags

	status RuntimeContextStatus

	parent *runtimeContextManager

	messageHandler Callable

	trackCpu         bool
	trackMem         bool
	trackTime        bool
	stopLevel        StopLevel
	startTime        uint64
	nextCpuThreshold uint64

	weakRefPool luagc.Pool
	poolFactory func() luagc.Pool
	gcPolicy    GCPolicy
}

var _ RuntimeContext = (*runtimeContextManager)(nil)

func (m *runtimeContextManager) initRoot() {
	m.gcPolicy = IsolateGCPolicy
	m.weakRefPool = m.poolFactory()
}

func (m *runtimeContextManager) HardLimits() RuntimeResources {
	return m.hardLimits
}

func (m *runtimeContextManager) SoftLimits() RuntimeResources {
	return m.softLimits
}

func (m *runtimeContextManager) UsedResources() RuntimeResources {
	return m.usedResources
}

func (m *runtimeContextManager) Status() RuntimeContextStatus {
	return m.status
}

func (m *runtimeContextManager) setStatus(st RuntimeContextStatus) {
	m.status = st
}

func (m *runtimeContextManager) RequiredFlags() ComplianceFlags {
	return m.requiredFlags
}

func (m *runtimeContextManager) CheckRequiredFlags(flags ComplianceFlags) error {
	missingFlags := m.requiredFlags &^ flags
	if missingFlags != 0 {
		return fmt.Errorf("missing flags: %s", strings.Join(missingFlags.Names(), " "))
	}
	return nil
}

func (m *runtimeContextManager) Parent() RuntimeContext {
	return m.parent
}

func (m *runtimeContextManager) SetStopLevel(stopLevel StopLevel) {
	m.stopLevel |= stopLevel
	if stopLevel&HardStop != 0 && m.status == StatusLive {
		m.KillContext()
	}
}

func (m *runtimeContextManager) Due() bool {
	return m.stopLevel&SoftStop != 0 || !m.softLimits.Dominates(m.usedResources)
}

func (m *runtimeContextManager) RuntimeContext() RuntimeContext {
	return m
}

func (m *runtimeContextManager) PushContext(ctx RuntimeContextDef) {
	if m.trackTime {
		m.updateTimeUsed()
	}
	parent := *m
	m.startTime = now()
	m.hardLimits = m.hardLimits.Remove(m.usedResources).Merge(ctx.HardLimits)
	m.softLimits = m.hardLimits.Merge(m.softLimits).Merge(ctx.SoftLimits)
	m.usedResources = RuntimeResources{}
	m.requiredFlags |= ctx.RequiredFlags

	if ctx.HardLimits.Cpu > 0 {
		m.requiredFlags |= ComplyCpuSafe
	}
	if ctx.HardLimits.Memory > 0 {
		m.requiredFlags |= ComplyMemSafe
	}
	if ctx.HardLimits.Millis > 0 {
		m.requiredFlags |= ComplyTimeSafe
	}
	m.trackTime = m.hardLimits.Millis > 0 || m.softLimits.Millis > 0
	m.trackCpu = m.hardLimits.Cpu > 0 || m.softLimits.Cpu > 0 || m.trackTime
	m.trackMem = m.hardLimits.Memory > 0 || m.softLimits.Memory > 0
	m.status = StatusLive
	m.messageHandler = ctx.MessageHandler
	m.parent = &parent
	if ctx.GCPolicy == IsolateGCPolicy || ctx.HardLimits.Millis > 0 || ctx.HardLimits.Cpu > 0 || ctx.HardLimits.Memory > 0 {
		m.weakRefPool = m.poolFactory()
		m.gcPolicy = IsolateGCPolicy
	} else {
		m.weakRefPool = parent.weakRefPool
		m.gcPolicy = ShareGCPolicy
	}
}

func (m *runtimeContextManager) GCPolicy() GCPolicy {
	return m.gcPolicy
}

func (m *runtimeContextManager) PopContext() RuntimeContext {
	if m == nil || m.parent == nil {
		return nil
	}
	if m.gcPolicy == IsolateGCPolicy {
		m.weakRefPool.ExtractAllMarkedFinalize()
		releaseResources(m.weakRefPool.ExtractAllMarkedRelease())
	}
	mCopy := *m
	if mCopy.status == StatusLive {
		mCopy.status = StatusDone
	}
	// Restore the parent as the current context BEFORE charging it the child's
	// consumption, rather than charging m.parent and restoring afterwards.
	// Charging a struct and then copying it is arithmetically identical to
	// copying it and then charging the copy, so this matters only when the
	// charge pushes the parent OVER a hard limit. RequireCPU/RequireMem then
	// panic, and panicking before `*m = *m.parent` would abandon the unwind
	// with the context stack still pointing at the already-popped child. A
	// pcall further out recovers into that misaligned stack, and its next
	// PushContext derives the child budget from an over-drawn parent:
	// RuntimeResources.Remove clamps the subtraction at 0, and a limit of 0
	// means "unlimited" in this encoding, so the limits would be gone
	// altogether. updateTimeUsed() below can panic for the same reason and
	// already sits after the restore.
	childCpuUsed := m.usedResources.Cpu
	childMemUsed := m.usedResources.Memory
	*m = *m.parent
	m.RequireCPU(childCpuUsed)
	m.RequireMem(childMemUsed)
	if m.trackTime {
		m.updateTimeUsed()
	}
	return &mCopy
}

func (m *runtimeContextManager) RequireCPU(cpuAmount uint64) {
	if m.trackCpu {
		// The path with limit is "outlined" so RequireCPU can be inlined,
		// minimising the overhead when there is no limit.
		m.requireCPU(cpuAmount)
	}
}

//go:noinline
func (m *runtimeContextManager) requireCPU(cpuAmount uint64) {
	if m.stopLevel&HardStop != 0 {
		m.KillContext()
	}
	cpuUsed := m.usedResources.Cpu + cpuAmount
	// Record the new total BEFORE the limit check, so the very tick that
	// exhausts the budget is still accounted for. Assigning it only on the
	// non-terminating path leaves a killed context reporting that it consumed
	// nothing, so PopContext charges its parent nothing -- which makes the
	// limit escapable at zero cost, because a script looping on pcall(f) then
	// gets a full fresh budget on every iteration however much f consumed.
	m.usedResources.Cpu = cpuUsed
	if atLimit(cpuUsed, m.hardLimits.Cpu) {
		m.TerminateContext("CPU limit of %d exceeded", m.hardLimits.Cpu)
	}
	if m.trackTime && m.nextCpuThreshold <= cpuUsed {
		m.nextCpuThreshold = cpuUsed + cpuThresholdIncrement
		m.updateTimeUsed()
	}
}

func (m *runtimeContextManager) UnusedCPU() uint64 {
	// usedResources may exceed hardLimits: requireCPU records the over-limit
	// total precisely so PopContext can charge it onward. This subtraction
	// would then wrap to a near-maximum uint64 that LinearUnused reads as an
	// enormous free budget. A hard limit of 0 means "unlimited" and must keep
	// returning the wrapped value, which is what callers already interpret as
	// unbounded -- hence the > 0 condition.
	if m.hardLimits.Cpu > 0 && m.usedResources.Cpu >= m.hardLimits.Cpu {
		return 0
	}
	return m.hardLimits.Cpu - m.usedResources.Cpu
}

func (m *runtimeContextManager) RequireMem(memAmount uint64) {
	if m.trackMem {
		// The path with limit is "outlined" so RequireMem can be inlined,
		// minimising the overhead when there is no limit.
		m.requireMem(memAmount)
	}
}

//go:noinline
func (m *runtimeContextManager) requireMem(memAmount uint64) {
	if m.stopLevel&HardStop != 0 {
		m.KillContext()
	}
	memUsed := m.usedResources.Memory + memAmount
	// Record the new total BEFORE the limit check, for the reason given in
	// requireCPU above: a killed context that reports consuming nothing gets
	// its parent charged nothing by PopContext, and the limit becomes escapable
	// at zero cost through a pcall loop.
	m.usedResources.Memory = memUsed
	if atLimit(memUsed, m.hardLimits.Memory) {
		m.TerminateContext("memory limit of %d exceeded", m.hardLimits.Memory)
	}
}

func (m *runtimeContextManager) RequireSize(sz uintptr) (mem uint64) {
	mem = uint64(sz)
	m.RequireMem(mem)
	return
}

func (m *runtimeContextManager) RequireArrSize(sz uintptr, n int) (mem uint64) {
	mem = uint64(sz) * uint64(n)
	m.RequireMem(mem)
	return
}

func (m *runtimeContextManager) RequireBytes(n int) (mem uint64) {
	mem = uint64(n)
	m.RequireMem(mem)
	return
}

func (m *runtimeContextManager) ReleaseMem(memAmount uint64) {
	// TODO: think about what to do when memory is released when unwinding from
	// a quota exceeded error
	if m.hardLimits.Memory > 0 {
		if memAmount <= m.usedResources.Memory {
			m.usedResources.Memory -= memAmount
		} else {
			panic("Too much mem released")
		}
	}
}

func (m *runtimeContextManager) ReleaseSize(sz uintptr) {
	m.ReleaseMem(uint64(sz))
}

func (m *runtimeContextManager) ReleaseArrSize(sz uintptr, n int) {
	m.ReleaseMem(uint64(sz) * uint64(n))
}

func (m *runtimeContextManager) ReleaseBytes(n int) {
	m.ReleaseMem(uint64(n))
}

func (m *runtimeContextManager) UnusedMem() uint64 {
	// Same wrap-around and same "a hard limit of 0 means unlimited" carve-out
	// as UnusedCPU above.
	if m.hardLimits.Memory > 0 && m.usedResources.Memory >= m.hardLimits.Memory {
		return 0
	}
	return m.hardLimits.Memory - m.usedResources.Memory
}

func (m *runtimeContextManager) updateTimeUsed() {
	m.usedResources.Millis = now() - m.startTime
	if atLimit(m.usedResources.Millis, m.hardLimits.Millis) {
		m.TerminateContext("time limit of %d exceeded", m.hardLimits.Millis)
	}
}

// LinearUnused returns an amount of resource combining memory and cpu.  It is
// useful when calling functions whose time complexity is a linear function of
// the size of their output.  As cpu ticks are "smaller" than memory ticks, the
// cpuFactor arguments allows specifying an increased "weight" for cpu ticks.
func (m *runtimeContextManager) LinearUnused(cpuFactor uint64) uint64 {
	mem := m.UnusedMem()
	cpu := m.UnusedCPU() * cpuFactor
	switch {
	case cpu == 0:
		return mem
	case mem == 0:
		return cpu
	case cpu > mem:
		return mem
	default:
		return cpu
	}
}

// LinearRequire can be used to actually consume (part of) the resource budget
// returned by LinearUnused (with the same cpuFactor).
func (m *runtimeContextManager) LinearRequire(cpuFactor uint64, amt uint64) {
	m.RequireMem(amt)
	m.RequireCPU(amt / cpuFactor)
}

// KillContext forcefully terminates the context with the message "force kill".
func (m *runtimeContextManager) KillContext() {
	m.TerminateContext("force kill")
}

// TerminateContext forcefully terminates the context with the given message.
func (m *runtimeContextManager) TerminateContext(format string, args ...interface{}) {
	if m.status != StatusLive {
		return
	}
	m.status = StatusKilled
	panic(ContextTerminationError{
		message: fmt.Sprintf(format, args...),
	})
}

// Current unix time in ms
func now() uint64 {
	return uint64(time.Now().UnixNano() / 1e6)
}
