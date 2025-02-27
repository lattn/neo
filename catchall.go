package neo

func (rg *RouteGroup) CatchAll(handlers ...Handler) *RouteGroup {
	hh := make([]Handler, len(rg.handlers)+len(handlers))
	copy(hh, rg.handlers)
	copy(hh[len(rg.handlers):], handlers)
	rg.router.catchAll.Insert(rg.prefix, hh)
	return rg
}
