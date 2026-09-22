-- Work that a Go function has already done when it reports its consumption
-- must be recorded even when it exhausts the budget, so that a killed child
-- context charges its parent for it and a script cannot get the same work for
-- free again by wrapping the call in pcall.
--
-- string.find hands the pattern matcher the context's remaining CPU as a
-- budget, the matcher uses it up, and only then is the consumption reported.
-- If that report is refused and dropped, the killed pcall child charges its
-- parent nothing and every iteration below gets a fresh budget.

local s = ("a"):rep(200)
local n = 0
print(runtime.callcontext({kill={cpu=100000}}, function()
    for _ = 1, 20 do
        n = n + 1
        pcall(string.find, s, "a*a*a*a*a*a*b")
    end
end))
--> =killed

-- The first pcall used up the whole budget, which kills the context.
print(n)
--> =1

-- The same through a nested callcontext with its own smaller limit: each
-- killed child charges its parent the budget it used up, so the parent runs
-- out after a bounded number of them.  This also exercises PopContext
-- terminating the parent it has just restored, two levels deep.
n = 0
print(runtime.callcontext({kill={cpu=100000}}, function()
    for _ = 1, 50 do
        n = n + 1
        pcall(runtime.callcontext, {kill={cpu=10000}}, string.find, s, "a*a*a*a*a*a*b")
    end
end))
--> =killed

print(n <= 11)
--> =true

-- A request that is refused before any work is done costs nothing.  pcall can
-- therefore still catch an over-budget allocation and the script carry on
-- with its remaining budget, as it does in standard Lua.
print(runtime.callcontext({kill={memory=256000}}, function()
    local ok, err = pcall(string.rep, "x", 200000000)
    print(ok, err)
    --> ~false\t.*memory limit of \d+ exceeded
    return "carried on"
end))
--> =done	carried on
