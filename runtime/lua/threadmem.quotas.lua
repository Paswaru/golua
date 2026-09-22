-- A coroutine's goroutine stack (charged in Thread.Start) and the frames it
-- runs are charged to the context current when they are created and released
-- to the context current when they are done with.  The two differ when a
-- coroutine is created in one context and run to completion in another, and
-- the releasing context may have recorded less than is released.  That must
-- not crash the runtime.

-- Goroutine stack: created in the outer context, finished in an inner one
-- that has used almost nothing.
print(runtime.callcontext({kill={memory=1000000}}, function()
    local co = coroutine.create(function() return 1 end)
    return runtime.callcontext({kill={memory=500000}}, coroutine.resume, co)
end))
--> =done	done	true	1

-- Frame registers: allocated during the first resume in one inner context,
-- released when the frame returns during the second resume in another.
local function big()
    local a1,a2,a3,a4,a5,a6,a7,a8,a9,a10,a11,a12,a13,a14,a15,a16,a17,a18,a19,a20
    local b1,b2,b3,b4,b5,b6,b7,b8,b9,b10,b11,b12,b13,b14,b15,b16,b17,b18,b19,b20
    local c1,c2,c3,c4,c5,c6,c7,c8,c9,c10,c11,c12,c13,c14,c15,c16,c17,c18,c19,c20
    local d1,d2,d3,d4,d5,d6,d7,d8,d9,d10,d11,d12,d13,d14,d15,d16,d17,d18,d19,d20
    local e1,e2,e3,e4,e5,e6,e7,e8,e9,e10,e11,e12,e13,e14,e15,e16,e17,e18,e19,e20
    local f1,f2,f3,f4,f5,f6,f7,f8,f9,f10,f11,f12,f13,f14,f15,f16,f17,f18,f19,f20
    local g1,g2,g3,g4,g5,g6,g7,g8,g9,g10,g11,g12,g13,g14,g15,g16,g17,g18,g19,g20
    local h1,h2,h3,h4,h5,h6,h7,h8,h9,h10,h11,h12,h13,h14,h15,h16,h17,h18,h19,h20
    coroutine.yield(1)
    return 2
end
print(runtime.callcontext({kill={memory=1000000}}, function()
    local co = coroutine.create(big)
    print(runtime.callcontext({kill={memory=500000}}, coroutine.resume, co))
    --> =done	true	1
    print(runtime.callcontext({kill={memory=500000}}, coroutine.resume, co))
    --> =done	true	2
end))
--> =done

-- A coroutine killed by a CPU limit in a context that also tracks memory ends
-- in that (killed) context, which has recorded almost no memory.
print(runtime.callcontext({kill={memory=1000000}}, function()
    local co = coroutine.create(function() while true do end end)
    print(runtime.callcontext({kill={cpu=1000}}, coroutine.resume, co))
    --> =killed
    print(coroutine.status(co))
    --> =dead
end))
--> =done
