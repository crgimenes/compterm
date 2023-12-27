package luaengine

import (
	"github.com/crgimenes/compterm/config"
	lua "github.com/yuin/gopher-lua"
)

func ConfigPath(L *lua.LState) int {
	L.Push(lua.LString(config.CFG.Path))
	return 1
}

func Startup(initLua string) error {
	L := lua.NewState()
	defer L.Close()
	L.SetGlobal("ConfigPath", L.NewFunction(ConfigPath))
	return L.DoFile(initLua)
}
