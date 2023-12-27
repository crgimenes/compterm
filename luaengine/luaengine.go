package luaengine

import (
	lua "github.com/yuin/gopher-lua"
)

func Startup(initLua string) error {
	L := lua.NewState()
	defer L.Close()
	return L.DoFile(initLua)
}
