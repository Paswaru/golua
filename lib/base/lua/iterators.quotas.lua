-- The iterators that ipairs and pairs push comply with all safety flags, so
-- iterating a table works inside a restricted context.  Their flags are
-- declared in init() rather than next to the SetEnvGoFunc calls in Load, so
-- check them here.

runtime.callcontext({flags="cpusafe memsafe timesafe iosafe"}, function ()
    local t = {5, 4, 3}

    local s = 0
    for i, v in ipairs(t) do
        s = s + i * v
    end
    print(s)
    --> =22

    local n = 0
    for k, v in pairs(t) do
        n = n + v
    end
    print(n)
    --> =12

    print(next({}))
    --> =nil
end)

-- pairs falls back on the function the "next" global is bound to.
print(pairs({}) == next)
--> =true
