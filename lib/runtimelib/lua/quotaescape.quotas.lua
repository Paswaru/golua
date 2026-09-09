-- A killed child context must charge what it consumed to its parent, so a
-- script cannot win a fresh budget by wrapping the expensive call in pcall.
--
-- pcall runs its argument in a child context. If the allocation that breaches
-- the limit goes unrecorded, the killed child reports consuming nothing,
-- PopContext charges the parent nothing, and every iteration below starts from
-- a clean budget.
--
-- Memory is the shape that escapes: one string.rep asks for 200MB at once, so
-- the single unrecorded request is the whole budget. CPU accrues in small
-- increments that are recorded as they go, which is why this loop is written
-- against a memory ceiling.

local n = 0
print(runtime.callcontext({kill={memory=256000}}, function()
    while true do
        n = n + 1
        pcall(function() return string.rep("x", 200000000) end)
    end
end))
--> =killed

-- One iteration exhausts the budget. Before the fix this ran into the
-- thousands, each pcall starting over from 256KB.
print(n < 10)
--> =true

-- Nesting pcall must not lose the ceiling either. PopContext restores the
-- parent before charging it, so the charge that pushes the parent over its own
-- limit terminates it with the context stack already correct. Charging before
-- restoring leaves the stack pointing at the popped child; an enclosing pcall
-- then recovers into it and derives the next budget from an over-drawn parent,
-- where the subtraction clamps at 0 and 0 encodes "unlimited" -- so the loop
-- below runs to completion with no ceiling at all.
--
-- Bounded rather than `while true` so that a regression fails the test quickly
-- instead of hanging it.
local d = 0
print(runtime.callcontext({kill={memory=256000}}, function()
    for _ = 1, 100 do
        d = d + 1
        pcall(function()
            pcall(function() return string.rep("x", 200000000) end)
        end)
    end
end))
--> =killed

print(d < 10)
--> =true
