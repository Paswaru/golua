package base

import rt "github.com/arnodel/golua/runtime"

// pairs returns the pairs implementation bound to nextGoFunc, the iterator it
// pushes when the collection has no __pairs metamethod. Load builds that
// iterator once per Runtime and passes it in here; see ipairs for why it is no
// longer a package-level var.
func pairs(nextGoFunc *rt.GoFunction) rt.GoFunctionFunc {
	return func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		if err := c.Check1Arg(); err != nil {
			return nil, err
		}
		coll := c.Arg(0)
		next := c.Next()
		res := rt.NewTerminationWith(c, 0, true)
		err, ok := rt.Metacall(t, coll, "__pairs", []rt.Value{coll}, res)
		if ok {
			if err != nil {
				return nil, err
			}
			t.Push(next, res.Etc()...)
			return next, nil
		}
		t.Push(next, rt.FunctionValue(nextGoFunc), coll, rt.NilValue)
		return next, nil
	}
}
